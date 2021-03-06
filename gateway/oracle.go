// +build evm

package gateway

import (
	"context"
	"encoding/hex"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/auth"
	"github.com/diademnetwork/go-diadem/client"
	lcrypto "github.com/diademnetwork/go-diadem/crypto"
	ltypes "github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/gateway/ethcontract"
	"github.com/pkg/errors"
)

type recentHashPool struct {
	hashMap         map[string]bool
	cleanupInterval time.Duration
	ticker          *time.Ticker
	stopCh          chan struct{}

	accessMutex sync.RWMutex
}

func newRecentHashPool(cleanupInterval time.Duration) *recentHashPool {
	return &recentHashPool{
		hashMap:         make(map[string]bool),
		cleanupInterval: cleanupInterval,
	}
}

func (r *recentHashPool) addHash(hash []byte) bool {
	r.accessMutex.Lock()
	defer r.accessMutex.Unlock()

	hexEncodedHash := hex.EncodeToString(hash)

	if _, ok := r.hashMap[hexEncodedHash]; ok {
		// If we are returning false, this means we have already seen hash
		return false
	}

	r.hashMap[hexEncodedHash] = true
	return true
}

func (r *recentHashPool) seenHash(hash []byte) bool {
	r.accessMutex.RLock()
	defer r.accessMutex.RUnlock()

	hexEncodedHash := hex.EncodeToString(hash)

	_, ok := r.hashMap[hexEncodedHash]
	return ok
}

func (r *recentHashPool) startCleanupRoutine() {
	r.ticker = time.NewTicker(r.cleanupInterval)
	r.stopCh = make(chan struct{})

	go func() {
		for {
			select {
			case <-r.stopCh:
				return
			case <-r.ticker.C:
				r.accessMutex.Lock()
				r.hashMap = make(map[string]bool)
				r.accessMutex.Unlock()
				break
			}
		}
	}()

}

func (r *recentHashPool) stopCleanupRoutine() {
	close(r.stopCh)
	r.ticker.Stop()
}

type mainnetEventInfo struct {
	BlockNum uint64
	TxIdx    uint
	Event    *MainnetEvent
}

type Status struct {
	Version                  string
	OracleAddress            string
	DAppChainGatewayAddress  string
	MainnetGatewayAddress    string
	NextMainnetBlockNum      uint64    `json:",string"`
	MainnetGatewayLastSeen   time.Time // TODO: hook this up
	DAppChainGatewayLastSeen time.Time
	// Number of Mainnet events submitted to the DAppChain Gateway successfully
	NumMainnetEventsFetched uint64 `json:",string"`
	// Total number of Mainnet events fetched
	NumMainnetEventsSubmitted uint64 `json:",string"`
}

type Oracle struct {
	cfg        TransferGatewayConfig
	chainID    string
	solGateway *ethcontract.MainnetGatewayContract
	goGateway  *DAppChainGateway
	startBlock uint64
	logger     *diadem.Logger
	ethClient  *MainnetClient
	address    diadem.Address
	// Used to sign tx/data sent to the DAppChain Gateway contract
	signer auth.Signer
	// Private key that should be used to sign tx/data sent to Mainnet Gateway contract
	mainnetPrivateKey     lcrypto.PrivateKey
	dAppChainPollInterval time.Duration
	mainnetPollInterval   time.Duration
	startupDelay          time.Duration
	reconnectInterval     time.Duration
	mainnetGatewayAddress diadem.Address

	numMainnetBlockConfirmations uint64
	numMainnetEventsFetched      uint64
	numMainnetEventsSubmitted    uint64

	statusMutex sync.RWMutex
	status      Status

	metrics *Metrics

	hashPool *recentHashPool

	isDiademCoinOracle      bool
	withdrawalSig         WithdrawalSigType
	withdrawerBlacklist   []diadem.Address
	receiptSigningEnabled bool
}

func CreateOracle(cfg *TransferGatewayConfig, chainID string) (*Oracle, error) {
	return createOracle(cfg, chainID, "tg_oracle", false)
}

func CreateDiademCoinOracle(cfg *TransferGatewayConfig, chainID string) (*Oracle, error) {
	return createOracle(cfg, chainID, "diadem_tg_oracle", true)
}

func createOracle(cfg *TransferGatewayConfig, chainID string, metricSubsystem string, isDiademCoinOracle bool) (*Oracle, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var signerType string

	privKey, err := LoadDAppChainPrivateKey(cfg.DappChainPrivateKeyHsmEnabled, cfg.DAppChainPrivateKeyPath)
	if err != nil {
		return nil, err
	}

	if cfg.DappChainPrivateKeyHsmEnabled {
		signerType = auth.SignerTypeYubiHsm
	} else {
		signerType = auth.SignerTypeEd25519
	}
	signer := auth.NewSigner(signerType, privKey)

	mainnetPrivateKey, err := LoadMainnetPrivateKey(cfg.MainnetPrivateKeyHsmEnabled, cfg.MainnetPrivateKeyPath)
	if err != nil {
		return nil, err
	}

	address := diadem.Address{
		ChainID: chainID,
		Local:   diadem.LocalAddressFromPublicKey(signer.PublicKey()),
	}

	if !common.IsHexAddress(cfg.MainnetContractHexAddress) {
		return nil, errors.New("invalid Mainnet Gateway address")
	}

	withdrawerBlacklist, err := cfg.GetWithdrawerAddressBlacklist()
	if err != nil {
		return nil, err
	}

	hashPool := newRecentHashPool(time.Duration(cfg.MainnetPollInterval) * time.Second * 4)
	hashPool.startCleanupRoutine()

	return &Oracle{
		cfg:                          *cfg,
		chainID:                      chainID,
		logger:                       diadem.NewDiademLogger(cfg.OracleLogLevel, cfg.OracleLogDestination),
		address:                      address,
		signer:                       signer,
		mainnetPrivateKey:            mainnetPrivateKey,
		dAppChainPollInterval:        time.Duration(cfg.DAppChainPollInterval) * time.Second,
		mainnetPollInterval:          time.Duration(cfg.MainnetPollInterval) * time.Second,
		numMainnetBlockConfirmations: uint64(cfg.NumMainnetBlockConfirmations),
		startupDelay:                 time.Duration(cfg.OracleStartupDelay) * time.Second,
		reconnectInterval:            time.Duration(cfg.OracleReconnectInterval) * time.Second,
		mainnetGatewayAddress: diadem.Address{
			ChainID: "eth",
			Local:   common.HexToAddress(cfg.MainnetContractHexAddress).Bytes(),
		},
		status: Status{
			Version:               diademchain.FullVersion(),
			OracleAddress:         address.String(),
			MainnetGatewayAddress: cfg.MainnetContractHexAddress,
		},

		metrics:             NewMetrics(metricSubsystem),
		hashPool:            hashPool,
		isDiademCoinOracle:    isDiademCoinOracle,
		withdrawalSig:       cfg.WithdrawalSig,
		withdrawerBlacklist: withdrawerBlacklist,
		// Oracle will do receipt signing when BatchSignFnConfig is disabled
		receiptSigningEnabled: !cfg.BatchSignFnConfig.Enabled,
	}, nil
}

// Status returns some basic info about the current state of the Oracle.
func (orc *Oracle) Status() *Status {
	orc.statusMutex.RLock()

	s := orc.status

	orc.statusMutex.RUnlock()
	return &s
}

func (orc *Oracle) updateStatus() {
	orc.statusMutex.Lock()

	orc.status.NextMainnetBlockNum = orc.startBlock
	orc.status.NumMainnetEventsFetched = orc.numMainnetEventsFetched
	orc.status.NumMainnetEventsSubmitted = orc.numMainnetEventsSubmitted

	if orc.goGateway != nil {
		orc.status.DAppChainGatewayAddress = orc.goGateway.Address.String()
		orc.status.DAppChainGatewayLastSeen = orc.goGateway.LastResponseTime
	}

	orc.statusMutex.Unlock()
}

func (orc *Oracle) connect() error {
	var err error

	if orc.ethClient == nil {
		orc.ethClient, err = ConnectToMainnet(orc.cfg.EthereumURI)
		if err != nil {
			return errors.Wrap(err, "failed to connect to Ethereum")
		}
	}

	if orc.solGateway == nil {
		orc.solGateway, err = ethcontract.NewMainnetGatewayContract(
			common.HexToAddress(orc.cfg.MainnetContractHexAddress),
			orc.ethClient,
		)
		if err != nil {
			return errors.Wrap(err, "failed create Mainnet Gateway contract binding")
		}
	}

	if orc.goGateway == nil {
		dappClient := client.NewDAppChainRPCClient(orc.chainID, orc.cfg.DAppChainWriteURI, orc.cfg.DAppChainReadURI)

		if orc.isDiademCoinOracle {
			orc.goGateway, err = ConnectToDAppChainDiademCoinGateway(dappClient, orc.address, orc.signer, orc.logger)
			if err != nil {
				return errors.Wrap(err, "failed to create dappchain diademcoin gateway")
			}
		} else {
			orc.goGateway, err = ConnectToDAppChainGateway(dappClient, orc.address, orc.signer, orc.logger)
			if err != nil {
				return errors.Wrap(err, "failed to create dappchain gateway")
			}
		}

	}
	return nil
}

// RunWithRecovery should run in a goroutine, it will ensure the oracle keeps on running as long
// as it doesn't panic due to a runtime error.
func (orc *Oracle) RunWithRecovery() {
	defer func() {
		if r := recover(); r != nil {
			orc.logger.Error("recovered from panic in Gateway Oracle", "r", r)
			// Unless it's a runtime error restart the goroutine
			if _, ok := r.(runtime.Error); !ok {
				time.Sleep(30 * time.Second)
				orc.logger.Info("Restarting Gateway Oracle...")
				go orc.RunWithRecovery()
			}
		}
	}()

	// When running in-process give the node a bit of time to spin up.
	if orc.startupDelay > 0 {
		time.Sleep(orc.startupDelay)
	}

	orc.Run()
}

// TODO: Graceful shutdown
func (orc *Oracle) Run() {
	for {
		if err := orc.connect(); err != nil {
			orc.logger.Error("[TG Oracle] failed to connect", "err", err)
			orc.updateStatus()
		} else {
			orc.updateStatus()
			break
		}
		time.Sleep(orc.reconnectInterval)
	}

	skipSleep := true
	for {
		if !skipSleep {
			time.Sleep(orc.mainnetPollInterval)
		} else {
			skipSleep = false
		}
		// TODO: should be possible to poll DAppChain & Mainnet at different intervals
		orc.pollMainnet()
		orc.pollDAppChain()
	}
}

func (orc *Oracle) pollMainnet() error {
	lastMainnetBlockNum, err := orc.goGateway.LastMainnetBlockNum()
	if err != nil {
		return err
	}

	startBlock := lastMainnetBlockNum + 1
	if orc.startBlock > startBlock {
		startBlock = orc.startBlock
	}

	// TODO: limit max block range per batch
	latestBlock, err := orc.getLatestEthBlockNumber()
	if err != nil {
		orc.logger.Error("failed to obtain latest Ethereum block number", "err", err)
		return err
	}

	// Don't process a block until it's been confirmed
	if latestBlock <= orc.numMainnetBlockConfirmations {
		return nil
	}
	latestBlock -= orc.numMainnetBlockConfirmations

	if latestBlock < startBlock {
		// Wait for Ethereum to produce a new block...
		return nil
	}

	events, err := orc.fetchEvents(startBlock, latestBlock)
	if err != nil {
		orc.logger.Error("failed to fetch events from Ethereum", "err", err)
		return err
	}

	if len(events) > 0 {
		orc.numMainnetEventsFetched = orc.numMainnetEventsFetched + uint64(len(events))
		orc.updateStatus()

		if err := orc.goGateway.ProcessEventBatch(events); err != nil {
			return err
		}

		orc.numMainnetEventsSubmitted = orc.numMainnetEventsSubmitted + uint64(len(events))
		orc.metrics.SubmittedMainnetEvents(len(events))
		orc.updateStatus()
	}

	orc.startBlock = latestBlock + 1
	return nil
}

func (orc *Oracle) pollDAppChain() error {
	if err := orc.verifyContractCreators(); err != nil {
		return err
	}

	if orc.receiptSigningEnabled {
		// TODO: should probably just log errors and soldier on
		if err := orc.signPendingWithdrawals(); err != nil {
			return err
		}
	}
	return nil
}

func (orc *Oracle) filterSeenWithdrawals(withdrawals []*PendingWithdrawalSummary) []*PendingWithdrawalSummary {
	unseenWithdrawals := make([]*PendingWithdrawalSummary, len(withdrawals))

	currentIndex := 0
	for _, withdrawal := range withdrawals {
		if !orc.hashPool.addHash(withdrawal.Hash) {
			continue
		}

		unseenWithdrawals[currentIndex] = withdrawal
		currentIndex++
	}

	return unseenWithdrawals[:currentIndex]
}

func (orc *Oracle) signPendingWithdrawals() error {
	var err error
	var numWithdrawalsSigned int
	defer func(begin time.Time) {
		orc.metrics.MethodCalled(begin, "signPendingWithdrawals", err)
		orc.metrics.WithdrawalsSigned(numWithdrawalsSigned)
		orc.updateStatus()
	}(time.Now())

	withdrawals, err := orc.goGateway.PendingWithdrawals(orc.mainnetGatewayAddress)
	if err != nil {
		return err
	}

	// Filter already seen withdrawals in 4 * pollInterval time
	filteredWithdrawals := orc.filterSeenWithdrawals(withdrawals)

	for _, summary := range filteredWithdrawals {
		tokenOwner := diadem.UnmarshalAddressPB(summary.TokenOwner)

		skipWithdrawal := false
		for i := range orc.withdrawerBlacklist {
			if orc.withdrawerBlacklist[i].Compare(tokenOwner) == 0 {
				orc.logger.Info(
					"Withdrawer is blacklisted, won't sign withdrawal",
					"tokenOwner", tokenOwner.String(),
					"hash", hex.EncodeToString(summary.Hash),
				)
				skipWithdrawal = true
				break
			}
		}

		if skipWithdrawal {
			continue
		}

		sig, err := orc.signTransferGatewayWithdrawal(summary.Hash)
		if err != nil {
			return err
		}
		req := &ConfirmWithdrawalReceiptRequest{
			TokenOwner:      summary.TokenOwner,
			OracleSignature: sig,
			WithdrawalHash:  summary.Hash,
		}
		// Ignore errors indicating a receipt has been signed already, they simply indicate another
		// Oracle has managed to sign the receipt already.
		// TODO: replace hardcoded error message with gateway.ErrWithdrawalReceiptSigned when this
		//       code is moved back into diademchain
		if err = orc.goGateway.ConfirmWithdrawalReceipt(req); err != nil {
			if strings.HasPrefix(err.Error(), "TG006:") {
				orc.logger.Debug("withdrawal already signed",
					"tokenOwner", tokenOwner.String(),
					"hash", hex.EncodeToString(summary.Hash),
				)
				err = nil
			} else {
				return err
			}
		} else {
			numWithdrawalsSigned++
			orc.logger.Debug("submitted signed withdrawal to DAppChain",
				"tokenOwner", tokenOwner.String(),
				"hash", hex.EncodeToString(summary.Hash),
			)
		}
	}
	return nil
}

func (orc *Oracle) verifyContractCreators() error {
	var err error
	var numContractCreatorsVerified int
	defer func(begin time.Time) {
		orc.metrics.MethodCalled(begin, "verifyContractCreators", err)
		orc.metrics.ContractCreatorsVerified(numContractCreatorsVerified)
		orc.updateStatus()
	}(time.Now())

	unverifiedCreators, err := orc.goGateway.UnverifiedContractCreators()
	if err != nil {
		return err
	}

	if len(unverifiedCreators) == 0 {
		return nil
	}

	verifiedCreators := make([]*VerifiedContractCreator, 0, len(unverifiedCreators))
	for _, unverifiedCreator := range unverifiedCreators {
		verifiedCreator, err := orc.fetchMainnetContractCreator(unverifiedCreator)
		if err != nil {
			orc.logger.Debug("failed to fetch Mainnet contract creator", "err", err)
		} else {
			verifiedCreators = append(verifiedCreators, verifiedCreator)
			numContractCreatorsVerified++
		}
	}

	err = orc.goGateway.VerifyContractCreators(verifiedCreators)
	return err
}

func (orc *Oracle) fetchMainnetContractCreator(unverified *UnverifiedContractCreator) (*VerifiedContractCreator, error) {
	verifiedCreator := &VerifiedContractCreator{
		ContractMappingID: unverified.ContractMappingID,
		Creator:           diadem.RootAddress("eth").MarshalPB(),
		Contract:          diadem.RootAddress("eth").MarshalPB(),
	}
	txHash := common.BytesToHash(unverified.ContractTxHash)
	tx, err := orc.ethClient.ContractCreationTxByHash(context.TODO(), txHash)
	if err == ethereum.NotFound {
		return verifiedCreator, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "failed to find contract creator by tx hash %v", txHash)
	}
	verifiedCreator.Creator.Local = diadem.LocalAddress(tx.CreatorAddress.Bytes())
	verifiedCreator.Contract.Local = diadem.LocalAddress(tx.ContractAddress.Bytes())
	return verifiedCreator, nil
}

func (orc *Oracle) getLatestEthBlockNumber() (uint64, error) {
	blockHeader, err := orc.ethClient.HeaderByNumber(context.TODO(), nil)
	if err != nil {
		return 0, err
	}
	return blockHeader.Number.Uint64(), nil
}

// Fetches all relevant events from an Ethereum node from startBlock to endBlock (inclusive)
func (orc *Oracle) fetchEvents(startBlock, endBlock uint64) ([]*MainnetEvent, error) {
	// NOTE: Currently either all blocks from w.StartBlock are processed successfully or none are.
	filterOpts := &bind.FilterOpts{
		Start: startBlock,
		End:   &endBlock,
	}

	var erc721Deposits, erc721xDeposits, diademcoinDeposits, erc20Deposits, ethDeposits, withdrawals []*mainnetEventInfo
	var err error

	// This is required, as DiademCoin gateway fires both erc20 as well as diademcoin received event
	if orc.isDiademCoinOracle {
		diademcoinDeposits, err = orc.fetchDiademCoinDeposits(filterOpts)
		if err != nil {
			return nil, err
		}
	} else {
		erc721Deposits, err = orc.fetchERC721Deposits(filterOpts)
		if err != nil {
			return nil, err
		}

		erc721xDeposits, err = orc.fetchERC721XDeposits(filterOpts)
		if err != nil {
			return nil, err
		}

		erc20Deposits, err = orc.fetchERC20Deposits(filterOpts)
		if err != nil {
			return nil, err
		}

		ethDeposits, err = orc.fetchETHDeposits(filterOpts)
		if err != nil {
			return nil, err
		}
	}

	withdrawals, err = orc.fetchTokenWithdrawals(filterOpts)
	if err != nil {
		return nil, err
	}

	events := make(
		[]*mainnetEventInfo, 0,
		len(erc721Deposits)+len(erc721xDeposits)+len(erc20Deposits)+len(ethDeposits)+len(diademcoinDeposits)+len(withdrawals),
	)
	events = append(erc721Deposits, erc721xDeposits...)
	events = append(events, erc20Deposits...)
	events = append(events, ethDeposits...)
	events = append(events, diademcoinDeposits...)
	events = append(events, withdrawals...)
	sortMainnetEvents(events)
	sortedEvents := make([]*MainnetEvent, len(events))
	for i, event := range events {
		sortedEvents[i] = event.Event
	}

	if len(events) > 0 {
		orc.logger.Debug("fetched Mainnet events",
			"startBlock", startBlock,
			"endBlock", endBlock,
			"erc721-deposits", len(erc721Deposits),
			"erc721x-deposits", len(erc721xDeposits),
			"erc20-deposits", len(erc20Deposits),
			"eth-deposits", len(ethDeposits),
			"diademcoin-deposits", len(diademcoinDeposits),
			"withdrawals", len(withdrawals),
		)
	}

	return sortedEvents, nil
}

func sortMainnetEvents(events []*mainnetEventInfo) {
	// Sort events by block & tx index (within the block)
	sort.Slice(events, func(i, j int) bool {
		if events[i].BlockNum == events[j].BlockNum {
			return events[i].TxIdx < events[j].TxIdx
		}
		return events[i].BlockNum < events[j].BlockNum
	})
}

func (orc *Oracle) fetchERC721Deposits(filterOpts *bind.FilterOpts) ([]*mainnetEventInfo, error) {
	var err error
	var numEvents int

	defer func(begin time.Time) {
		orc.metrics.MethodCalled(begin, "fetchERC721Deposits", err)
		orc.metrics.FetchedMainnetEvents(numEvents, "ERC721Received")
	}(time.Now())

	erc721It, err := orc.solGateway.FilterERC721Received(filterOpts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get logs for ERC721Received")
	}
	events := []*mainnetEventInfo{}
	for {
		ok := erc721It.Next()
		if ok {
			ev := erc721It.Event
			tokenAddr, err := diadem.LocalAddressFromHexString(ev.ContractAddress.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse ERC721Received token address")
			}
			fromAddr, err := diadem.LocalAddressFromHexString(ev.From.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse ERC721Received from address")
			}
			events = append(events, &mainnetEventInfo{
				BlockNum: ev.Raw.BlockNumber,
				TxIdx:    ev.Raw.TxIndex,
				Event: &MainnetEvent{
					EthBlock: ev.Raw.BlockNumber,
					Payload: &MainnetDepositEvent{
						Deposit: &MainnetTokenDeposited{
							TokenKind:     TokenKind_ERC721,
							TokenContract: diadem.Address{ChainID: "eth", Local: tokenAddr}.MarshalPB(),
							TokenOwner:    diadem.Address{ChainID: "eth", Local: fromAddr}.MarshalPB(),
							TokenID:       &ltypes.BigUInt{Value: *diadem.NewBigUInt(ev.TokenId)},
							TxHash:        ev.Raw.TxHash.Bytes(),
						},
					},
				},
			})
		} else {
			err = erc721It.Error()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get event data for ERC721Received")
			}
			erc721It.Close()
			break
		}
	}
	numEvents = len(events)
	return events, nil
}

func (orc *Oracle) fetchERC721XDeposits(filterOpts *bind.FilterOpts) ([]*mainnetEventInfo, error) {
	var err error
	var numEvents int
	defer func(begin time.Time) {
		orc.metrics.MethodCalled(begin, "fetchERC721XDeposits", err)
		orc.metrics.FetchedMainnetEvents(numEvents, "ERC721XReceived")
	}(time.Now())

	it, err := orc.solGateway.FilterERC721XReceived(filterOpts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get logs for ERC721XReceived")
	}
	events := []*mainnetEventInfo{}
	for {
		ok := it.Next()
		if ok {
			ev := it.Event
			tokenAddr, err := diadem.LocalAddressFromHexString(ev.ContractAddress.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse ERC721XReceived token address")
			}
			fromAddr, err := diadem.LocalAddressFromHexString(ev.From.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse ERC721XReceived from address")
			}
			events = append(events, &mainnetEventInfo{
				BlockNum: ev.Raw.BlockNumber,
				TxIdx:    ev.Raw.TxIndex,
				Event: &MainnetEvent{
					EthBlock: ev.Raw.BlockNumber,
					Payload: &MainnetDepositEvent{
						Deposit: &MainnetTokenDeposited{
							TokenKind:     TokenKind_ERC721X,
							TokenContract: diadem.Address{ChainID: "eth", Local: tokenAddr}.MarshalPB(),
							TokenOwner:    diadem.Address{ChainID: "eth", Local: fromAddr}.MarshalPB(),
							TokenID:       &ltypes.BigUInt{Value: *diadem.NewBigUInt(ev.TokenId)},
							TokenAmount:   &ltypes.BigUInt{Value: *diadem.NewBigUInt(ev.Amount)},
							TxHash:        ev.Raw.TxHash.Bytes(),
						},
					},
				},
			})
		} else {
			err = it.Error()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get event data for ERC721XReceived")
			}
			it.Close()
			break
		}
	}
	numEvents = len(events)
	return events, nil
}

func (orc *Oracle) fetchERC20Deposits(filterOpts *bind.FilterOpts) ([]*mainnetEventInfo, error) {
	var err error
	var numEvents int
	defer func(begin time.Time) {
		orc.metrics.MethodCalled(begin, "fetchERC20Deposits", err)
		orc.metrics.FetchedMainnetEvents(numEvents, "ERC20Received")
	}(time.Now())

	it, err := orc.solGateway.FilterERC20Received(filterOpts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get logs for ERC20Received")
	}
	events := []*mainnetEventInfo{}
	for {
		ok := it.Next()
		if ok {
			ev := it.Event
			tokenAddr, err := diadem.LocalAddressFromHexString(ev.ContractAddress.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse ERC20Received token address")
			}
			fromAddr, err := diadem.LocalAddressFromHexString(ev.From.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse ERC20Received from address")
			}
			events = append(events, &mainnetEventInfo{
				BlockNum: ev.Raw.BlockNumber,
				TxIdx:    ev.Raw.TxIndex,
				Event: &MainnetEvent{
					EthBlock: ev.Raw.BlockNumber,
					Payload: &MainnetDepositEvent{
						Deposit: &MainnetTokenDeposited{
							TokenKind:     TokenKind_ERC20,
							TokenContract: diadem.Address{ChainID: "eth", Local: tokenAddr}.MarshalPB(),
							TokenOwner:    diadem.Address{ChainID: "eth", Local: fromAddr}.MarshalPB(),
							TokenAmount:   &ltypes.BigUInt{Value: *diadem.NewBigUInt(ev.Amount)},
							TxHash:        ev.Raw.TxHash.Bytes(),
						},
					},
				},
			})
		} else {
			err = it.Error()
			if err != nil {
				return nil, errors.Wrap(err, "Failed to get event data for ERC20Received")
			}
			it.Close()
			break
		}
	}
	numEvents = len(events)
	return events, nil
}

func (orc *Oracle) fetchDiademCoinDeposits(filterOpts *bind.FilterOpts) ([]*mainnetEventInfo, error) {
	var err error
	var numEvents int
	defer func(begin time.Time) {
		orc.metrics.MethodCalled(begin, "fetchDiademCoinDeposits", err)
		orc.metrics.FetchedMainnetEvents(numEvents, "DiademCoinReceived")
	}(time.Now())

	it, err := orc.solGateway.FilterDiademCoinReceived(filterOpts, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get logs for DiademCoinReceived")
	}
	events := []*mainnetEventInfo{}
	for {
		ok := it.Next()
		if ok {
			ev := it.Event
			tokenAddr, err := diadem.LocalAddressFromHexString(ev.DiademCoinAddress.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse DiademCoinReceived token address")
			}
			fromAddr, err := diadem.LocalAddressFromHexString(ev.From.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse DiademCoinReceived from address")
			}
			events = append(events, &mainnetEventInfo{
				BlockNum: ev.Raw.BlockNumber,
				TxIdx:    ev.Raw.TxIndex,
				Event: &MainnetEvent{
					EthBlock: ev.Raw.BlockNumber,
					Payload: &MainnetDepositEvent{
						Deposit: &MainnetTokenDeposited{
							TokenKind:     TokenKind_DiademCoin,
							TokenContract: diadem.Address{ChainID: "eth", Local: tokenAddr}.MarshalPB(),
							TokenOwner:    diadem.Address{ChainID: "eth", Local: fromAddr}.MarshalPB(),
							TokenAmount:   &ltypes.BigUInt{Value: *diadem.NewBigUInt(ev.Amount)},
							TxHash:        ev.Raw.TxHash.Bytes(),
						},
					},
				},
			})
		} else {
			err = it.Error()
			if err != nil {
				return nil, errors.Wrap(err, "Failed to get event data for DiademCoinReceived")
			}
			it.Close()
			break
		}
	}
	numEvents = len(events)
	return events, nil
}

func (orc *Oracle) fetchETHDeposits(filterOpts *bind.FilterOpts) ([]*mainnetEventInfo, error) {
	var err error
	var numEvents int
	defer func(begin time.Time) {
		orc.metrics.MethodCalled(begin, "fetchETHDeposits", err)
		orc.metrics.FetchedMainnetEvents(numEvents, "ETHReceived")
	}(time.Now())

	it, err := orc.solGateway.FilterETHReceived(filterOpts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get logs for ETHReceived")
	}
	events := []*mainnetEventInfo{}
	for {
		ok := it.Next()
		if ok {
			ev := it.Event
			fromAddr, err := diadem.LocalAddressFromHexString(ev.From.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse ETHReceived from address")
			}
			events = append(events, &mainnetEventInfo{
				BlockNum: ev.Raw.BlockNumber,
				TxIdx:    ev.Raw.TxIndex,
				Event: &MainnetEvent{
					EthBlock: ev.Raw.BlockNumber,
					Payload: &MainnetDepositEvent{
						Deposit: &MainnetTokenDeposited{
							TokenKind:   TokenKind_ETH,
							TokenOwner:  diadem.Address{ChainID: "eth", Local: fromAddr}.MarshalPB(),
							TokenAmount: &ltypes.BigUInt{Value: *diadem.NewBigUInt(ev.Amount)},
							TxHash:      ev.Raw.TxHash.Bytes(),
						},
					},
				},
			})
		} else {
			err = it.Error()
			if err != nil {
				return nil, errors.Wrap(err, "Failed to get event data for ETHReceived")
			}
			it.Close()
			break
		}
	}
	numEvents = len(events)
	return events, nil
}

func (orc *Oracle) fetchTokenWithdrawals(filterOpts *bind.FilterOpts) ([]*mainnetEventInfo, error) {
	var err error
	var numEvents int
	defer func(begin time.Time) {
		orc.metrics.MethodCalled(begin, "fetchTokenWithdrawals", err)
		orc.metrics.FetchedMainnetEvents(numEvents, "TokenWithdrawn")
	}(time.Now())

	it, err := orc.solGateway.FilterTokenWithdrawn(filterOpts, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get logs for TokenWithdrawn")
	}
	events := []*mainnetEventInfo{}
	for {
		ok := it.Next()
		if ok {
			ev := it.Event

			// Not strictly required, but will provide additional protection to oracle in case
			// we get any erc20 events from diademcoin gateway
			if orc.isDiademCoinOracle != (TokenKind(ev.Kind) == TokenKind_DiademCoin) {
				continue
			}

			tokenAddr, err := diadem.LocalAddressFromHexString(ev.ContractAddress.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse TokenWithdrawn token address")
			}
			fromAddr, err := diadem.LocalAddressFromHexString(ev.Owner.Hex())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse TokenWithdrawn from address")
			}

			var tokenID *ltypes.BigUInt
			var amount *ltypes.BigUInt
			switch TokenKind(ev.Kind) {
			case TokenKind_ERC721:
				tokenID = &ltypes.BigUInt{Value: *diadem.NewBigUInt(ev.Value)}
			// TODO: ERC721X TokenWithdrawn event should probably indicate the token ID... but for
			//       now all we have is the amount.
			case TokenKind_ERC721X, TokenKind_ERC20, TokenKind_ETH, TokenKind_DiademCoin:
				amount = &ltypes.BigUInt{Value: *diadem.NewBigUInt(ev.Value)}
			}

			events = append(events, &mainnetEventInfo{
				BlockNum: ev.Raw.BlockNumber,
				TxIdx:    ev.Raw.TxIndex,
				Event: &MainnetEvent{
					EthBlock: ev.Raw.BlockNumber,
					Payload: &MainnetWithdrawalEvent{
						Withdrawal: &MainnetTokenWithdrawn{
							TokenKind:     TokenKind(ev.Kind),
							TokenContract: diadem.Address{ChainID: "eth", Local: tokenAddr}.MarshalPB(),
							TokenOwner:    diadem.Address{ChainID: "eth", Local: fromAddr}.MarshalPB(),
							TokenID:       tokenID,
							TokenAmount:   amount,
							TxHash:        ev.Raw.TxHash.Bytes(),
						},
					},
				},
			})
		} else {
			err = it.Error()
			if err != nil {
				return nil, errors.Wrap(err, "Failed to get event data for TokenWithdrawn")
			}
			it.Close()
			break
		}
	}
	numEvents = len(events)
	return events, nil
}

func (orc *Oracle) signTransferGatewayWithdrawal(hash []byte) ([]byte, error) {
	var sig []byte
	var err error
	if orc.withdrawalSig == UnprefixedWithdrawalSigType {
		sig, err = lcrypto.SoliditySign(hash, orc.mainnetPrivateKey)
	} else if orc.withdrawalSig == PrefixedWithdrawalSigType {
		sig, err = lcrypto.SoliditySignPrefixed(hash, orc.mainnetPrivateKey)
	} else {
		return nil, errors.New("invalid withdrawal sig type")
	}

	if err != nil {
		return nil, err
	}
	// The first byte should be the signature mode, for details about the signature format refer to
	// https://github.com/diademnetwork/plasma-erc721/blob/master/server/contracts/Libraries/ECVerify.sol
	return append(make([]byte, 1, 66), sig...), nil
}

func LoadDAppChainPrivateKey(hsmEnabled bool, path string) (lcrypto.PrivateKey, error) {
	var privKey lcrypto.PrivateKey
	var err error

	if hsmEnabled {
		privKey, err = lcrypto.LoadYubiHsmPrivKey(path)
	} else {
		privKey, err = lcrypto.LoadEd25519PrivKey(path)
	}

	if err != nil {
		return nil, err
	}
	return privKey, nil
}

func LoadMainnetPrivateKey(hsmEnabled bool, path string) (lcrypto.PrivateKey, error) {
	var privKey lcrypto.PrivateKey
	var err error

	if hsmEnabled {
		privKey, err = lcrypto.LoadYubiHsmPrivKey(path)
	} else {
		privKey, err = lcrypto.LoadSecp256k1PrivKey(path)
	}

	if err != nil {
		return nil, err
	}
	return privKey, nil
}
