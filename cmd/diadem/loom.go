package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/push"

	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	"github.com/gogo/protobuf/proto"
	diadem "github.com/diademnetwork/go-diadem"
	glAuth "github.com/diademnetwork/go-diadem/auth"
	"github.com/diademnetwork/go-diadem/builtin/commands"
	"github.com/diademnetwork/go-diadem/cli"
	"github.com/diademnetwork/go-diadem/crypto"
	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/go-diadem/util"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/abci/backend"
	"github.com/diademnetwork/diademchain/auth"
	"github.com/diademnetwork/diademchain/builtin/plugins/dposv2"
	d2Oracle "github.com/diademnetwork/diademchain/builtin/plugins/dposv2/oracle"
	d2OracleCfg "github.com/diademnetwork/diademchain/builtin/plugins/dposv2/oracle/config"
	"github.com/diademnetwork/diademchain/builtin/plugins/dposv3"
	plasmaConfig "github.com/diademnetwork/diademchain/builtin/plugins/plasma_cash/config"
	plasmaOracle "github.com/diademnetwork/diademchain/builtin/plugins/plasma_cash/oracle"
	"github.com/diademnetwork/diademchain/receipts/leveldb"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/diademnetwork/diademchain/chainconfig"
	chaincfgcmd "github.com/diademnetwork/diademchain/cmd/diadem/chainconfig"
	"github.com/diademnetwork/diademchain/cmd/diadem/common"
	dbcmd "github.com/diademnetwork/diademchain/cmd/diadem/db"
	"github.com/diademnetwork/diademchain/cmd/diadem/dbg"
	deployer "github.com/diademnetwork/diademchain/cmd/diadem/deployerwhitelist"
	gatewaycmd "github.com/diademnetwork/diademchain/cmd/diadem/gateway"
	"github.com/diademnetwork/diademchain/cmd/diadem/replay"
	"github.com/diademnetwork/diademchain/cmd/diadem/staking"
	"github.com/diademnetwork/diademchain/config"
	"github.com/diademnetwork/diademchain/core"
	cdb "github.com/diademnetwork/diademchain/db"
	"github.com/diademnetwork/diademchain/eth/polls"
	"github.com/diademnetwork/diademchain/events"
	"github.com/diademnetwork/diademchain/evm"
	tgateway "github.com/diademnetwork/diademchain/gateway"
	karma_handler "github.com/diademnetwork/diademchain/karma"
	"github.com/diademnetwork/diademchain/log"
	"github.com/diademnetwork/diademchain/migrations"
	"github.com/diademnetwork/diademchain/plugin"
	"github.com/diademnetwork/diademchain/receipts"
	"github.com/diademnetwork/diademchain/receipts/handler"
	regcommon "github.com/diademnetwork/diademchain/registry"
	registry "github.com/diademnetwork/diademchain/registry/factory"
	"github.com/diademnetwork/diademchain/rpc"
	"github.com/diademnetwork/diademchain/store"
	"github.com/diademnetwork/diademchain/throttle"
	"github.com/diademnetwork/diademchain/tx_handler"
	"github.com/diademnetwork/diademchain/vm"
	"github.com/pkg/errors"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ed25519"

	"github.com/diademnetwork/diademchain/fnConsensus"
	dbm "github.com/tendermint/tendermint/libs/db"
)

var RootCmd = &cobra.Command{
	Use:   "diadem",
	Short: "Diadem DAppChain",
}

var codeLoaders map[string]core.ContractCodeLoader

func init() {
	codeLoaders = core.GetDefaultCodeLoaders()
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the Diadem chain version",
		RunE: func(cmd *cobra.Command, args []string) error {
			println(diademchain.FullVersion())
			return nil
		},
	}
}

func printEnv(env map[string]string) {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		val := env[key]
		fmt.Printf("%s = %s\n", key, val)
	}
}

func newEnvCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Show diadem config settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := common.ParseConfig()
			if err != nil {
				return err
			}

			printEnv(map[string]string{
				"version":       diademchain.FullVersion(),
				"build":         diademchain.Build,
				"build variant": diademchain.BuildVariant,
				"git sha":       diademchain.GitSHA,
				"go-diadem":       diademchain.GoDiademGitSHA,
				"go-ethereum":   diademchain.EthGitSHA,
				"go-plugin":     diademchain.HashicorpGitSHA,
				"plugin path":   cfg.PluginsPath(),
				"peers":         cfg.Peers,
			})
			return nil
		},
	}
}

type genKeyFlags struct {
	PublicFile string `json:"publicfile"`
	PrivFile   string `json:"privfile"`
}

func newGenKeyCommand() *cobra.Command {
	var flags genKeyFlags
	keygenCmd := &cobra.Command{
		Use:   "genkey",
		Short: "generate a public and private key pair",
		RunE: func(cmd *cobra.Command, args []string) error {
			pub, priv, err := ed25519.GenerateKey(nil)
			if err != nil {
				return fmt.Errorf("Error generating key pair: %v", err)
			}
			encoder := base64.StdEncoding
			pubKeyB64 := encoder.EncodeToString(pub[:])
			privKeyB64 := encoder.EncodeToString(priv[:])

			if err := ioutil.WriteFile(flags.PublicFile, []byte(pubKeyB64), 0664); err != nil {
				return fmt.Errorf("Unable to write public key: %v", err)
			}
			if err := ioutil.WriteFile(flags.PrivFile, []byte(privKeyB64), 0664); err != nil {
				return fmt.Errorf("Unable to write private key: %v", err)
			}
			addr := diadem.LocalAddressFromPublicKey(pub[:])
			fmt.Printf("local address: %s\n", addr.String())
			fmt.Printf("local address base64: %s\n", encoder.EncodeToString(addr))
			return nil
		},
	}
	keygenCmd.Flags().StringVarP(&flags.PublicFile, "public_key", "a", "", "public key file")
	keygenCmd.Flags().StringVarP(&flags.PrivFile, "private_key", "k", "", "private key file")
	return keygenCmd
}

type yubiHsmFlags struct {
	HsmNewKey  bool   `json:"newkey"`
	HsmLoadKey bool   `json:"loadkey"`
	HsmConfig  string `json:"config"`
}

func newYubiHsmCommand() *cobra.Command {
	var flags yubiHsmFlags
	keygenCmd := &cobra.Command{
		Use:   "yubihsm",
		Short: "generate or load YubiHSM ed25519/secp256k1 key",
		RunE: func(cmd *cobra.Command, args []string) error {
			var yubiPrivKey *crypto.YubiHsmPrivateKey
			var err error

			if len(flags.HsmConfig) == 0 {
				return fmt.Errorf("Please specify YubiHSM configuration file")
			}

			if !flags.HsmLoadKey {
				yubiPrivKey, err = crypto.GenYubiHsmPrivKey(flags.HsmConfig)
			} else {
				yubiPrivKey, err = crypto.LoadYubiHsmPrivKey(flags.HsmConfig)
			}
			if err != nil {
				return fmt.Errorf("Error generating or loading YubiHSM key: %v", err)
			}
			defer yubiPrivKey.UnloadYubiHsmPrivKey()

			fmt.Printf("Private Key Type:   %s\n", yubiPrivKey.GetKeyType())
			fmt.Printf("Private Key ID:     %d\n", yubiPrivKey.GetPrivKeyID())
			fmt.Printf("Public Key address: %s\n", yubiPrivKey.GetPubKeyAddr())

			b64addr, err := yubiPrivKey.GetPubKeyAddrB64Encoded()
			if err != nil {
				fmt.Printf("Public Key address base64-encoded: %v\n", err)
			} else {
				fmt.Printf("Public Key address base64-encoded: %s\n", b64addr)
			}

			return nil
		},
	}
	keygenCmd.Flags().BoolVarP(&flags.HsmNewKey, "new-key", "n", false, "generate YubiHSM ed25519/secp256k1 key")
	keygenCmd.Flags().BoolVarP(&flags.HsmLoadKey, "load-key", "l", false, "load YubiHSM ed25519/secp256k1 key")
	keygenCmd.Flags().StringVarP(&flags.HsmConfig, "hsm-config", "c", "", "yubihsm config")
	return keygenCmd
}

func newInitCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize configs and data",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := common.ParseConfig()
			if err != nil {
				return err
			}
			backend := initBackend(cfg, "", nil)
			if force {
				err = backend.Destroy()
				if err != nil {
					return err
				}
				err = destroyApp(cfg)
				if err != nil {
					return err
				}
				destroyReceiptsDB(cfg)
			}
			validator, err := backend.Init()
			if err != nil {
				return err
			}
			err = initApp(validator, cfg)
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "force initialization")
	return cmd
}

func newResetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset the app and blockchain state only",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := common.ParseConfig()
			if err != nil {
				return err
			}

			backend := initBackend(cfg, "", nil)
			err = backend.Reset(0)
			if err != nil {
				return err
			}

			err = resetApp(cfg)
			if err != nil {
				return err
			}

			destroyReceiptsDB(cfg)

			return nil
		},
	}
}

// Generate Or Prints node's ID to the standard output.
func newNodeKeyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "nodekey",
		Short: "Show node key",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := common.ParseConfig()
			if err != nil {
				return err
			}
			backend := initBackend(cfg, "", nil)
			key, err := backend.NodeKey()
			if err != nil {
				fmt.Printf("Error in determining Node Key")
			} else {
				fmt.Printf("%s\n", key)
			}
			return nil
		},
	}
}

func newRunCommand() *cobra.Command {
	var abciServerAddr string
	var appHeight int64

	cfg, err := common.ParseConfig()

	cmd := &cobra.Command{
		Use:   "run [root contract]",
		Short: "Run the blockchain node",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err != nil {
				return err
			}
			log.Setup(cfg.DiademLogLevel, cfg.LogDestination)
			logger := log.Default
			if cfg.PrometheusPushGateway.Enabled {
				host, err := os.Hostname()
				if err != nil {
					log.Error("Error in reporting Hostname by kernel", "Error", err)
					host = ""
				}
				go startPushGatewayMonitoring(cfg.PrometheusPushGateway, logger, host)
			}
			var fnRegistry fnConsensus.FnRegistry
			if cfg.FnConsensus.Enabled {
				fnRegistry = fnConsensus.NewInMemoryFnRegistry()
			}
			var loaders []plugin.Loader
			for _, loader := range cfg.ContractLoaders {
				if strings.EqualFold("static", loader) {
					loaders = append(loaders, common.NewDefaultContractsLoader(cfg))
				}
				if strings.EqualFold("dynamic", loader) {
					loaders = append(loaders, plugin.NewManager(cfg.PluginsPath()))
				}
				if strings.EqualFold("external", loader) {
					loaders = append(loaders, plugin.NewExternalLoader(cfg.PluginsPath()))
				}
			}
			backend := initBackend(cfg, abciServerAddr, fnRegistry)
			loader := plugin.NewMultiLoader(loaders...)
			termChan := make(chan os.Signal)
			go func(c <-chan os.Signal, l plugin.Loader) {
				<-c
				l.UnloadContracts()
				os.Exit(0)
			}(termChan, loader)

			signal.Notify(termChan, syscall.SIGHUP,
				syscall.SIGINT,
				syscall.SIGTERM,
				syscall.SIGQUIT)

			chainID, err := backend.ChainID()
			if err != nil {
				return err
			}

			app, err := loadApp(chainID, cfg, loader, backend, appHeight)
			if err != nil {
				return err
			}
			if err := backend.Start(app); err != nil {
				return err
			}

			nodeSigner, err := backend.NodeSigner()
			if err != nil {
				return err
			}

			if err := initQueryService(app, chainID, cfg, loader, app.ReceiptHandlerProvider); err != nil {
				return err
			}

			if err := startGatewayOracle(chainID, cfg.TransferGateway); err != nil {
				return err
			}

			if err := startGatewayFn(chainID, fnRegistry, cfg.TransferGateway, nodeSigner); err != nil {
				return err
			}

			if err := startDiademCoinGatewayOracle(chainID, cfg.DiademCoinTransferGateway); err != nil {
				return err
			}

			if err := startDiademCoinGatewayFn(chainID, fnRegistry, cfg.DiademCoinTransferGateway, nodeSigner); err != nil {
				return err
			}

			if err := startPlasmaOracle(chainID, cfg.PlasmaCash); err != nil {
				return err
			}

			if err := startDPOSv2Oracle(chainID, cfg.DPOSv2OracleConfig); err != nil {
				return err
			}

			if err := startFeatureAutoEnabler(chainID, cfg.ChainConfig, nodeSigner, backend, log.Default); err != nil {
				return err
			}

			backend.RunForever()

			return nil
		},
	}
	cmd.Flags().StringVarP(&cfg.Peers, "peers", "p", "", "peers")
	cmd.Flags().StringVar(&cfg.PersistentPeers, "persistent-peers", "", "persistent peers")
	cmd.Flags().StringVar(&abciServerAddr, "abci-server", "", "Serve ABCI app at specified address")
	cmd.Flags().Int64Var(&appHeight, "app-height", 0, "Start at the given block instead of the last block saved")
	return cmd
}

//nolint:deadcode
func recovery() {
	if r := recover(); r != nil {
		log.Error("caught RPC proxy exception, exiting", r)
		os.Exit(1)
	}
}

func startDPOSv2Oracle(chainID string, cfg *d2OracleCfg.OracleSerializableConfig) error {
	oracleCfg, err := d2OracleCfg.LoadSerializableConfig(chainID, cfg)
	if err != nil {
		return err
	}

	if !oracleCfg.Enabled {
		return nil
	}

	oracle := d2Oracle.NewOracle(oracleCfg)
	if err := oracle.Init(); err != nil {
		return err
	}

	oracle.Run()
	return nil
}

func startFeatureAutoEnabler(
	chainID string, cfg *config.ChainConfigConfig, nodeSigner glAuth.Signer, node backend.Backend,
	logger *diadem.Logger,
) error {
	if !cfg.AutoEnableFeatures || !cfg.ContractEnabled {
		return nil
	}

	routine, err := chainconfig.NewChainConfigRoutine(cfg, chainID, nodeSigner, node, logger)
	if err != nil {
		return err
	}

	go routine.RunWithRecovery()

	return nil
}

func startPlasmaOracle(chainID string, cfg *plasmaConfig.PlasmaCashSerializableConfig) error {
	plasmaCfg, err := plasmaConfig.LoadSerializableConfig(chainID, cfg)
	if err != nil {
		return err
	}

	if !plasmaCfg.OracleEnabled {
		return nil
	}

	oracle := plasmaOracle.NewOracle(plasmaCfg.OracleConfig)
	err = oracle.Init()
	if err != nil {
		return err
	}

	oracle.Run()

	return nil
}

func startGatewayFn(
	chainID string,
	fnRegistry fnConsensus.FnRegistry,
	cfg *tgateway.TransferGatewayConfig,
	nodeSigner glAuth.Signer,
) error {
	if !cfg.BatchSignFnConfig.Enabled {
		return nil
	}

	batchSignWithdrawalFn, err := tgateway.CreateBatchSignWithdrawalFn(false, chainID, fnRegistry, cfg, nodeSigner)
	if err != nil {
		return err
	}

	return fnRegistry.Set("batch_sign_withdrawal", batchSignWithdrawalFn)
}

func startDiademCoinGatewayFn(
	chainID string,
	fnRegistry fnConsensus.FnRegistry,
	cfg *tgateway.TransferGatewayConfig,
	nodeSigner glAuth.Signer,
) error {
	if !cfg.BatchSignFnConfig.Enabled {
		return nil
	}

	batchSignWithdrawalFn, err := tgateway.CreateBatchSignWithdrawalFn(true, chainID, fnRegistry, cfg, nodeSigner)
	if err != nil {
		return err
	}

	return fnRegistry.Set("diademcoin:batch_sign_withdrawal", batchSignWithdrawalFn)
}

func startDiademCoinGatewayOracle(chainID string, cfg *tgateway.TransferGatewayConfig) error {
	if !cfg.OracleEnabled {
		return nil
	}

	orc, err := tgateway.CreateDiademCoinOracle(cfg, chainID)
	if err != nil {
		return err
	}

	go orc.RunWithRecovery()
	return nil
}

func startGatewayOracle(chainID string, cfg *tgateway.TransferGatewayConfig) error {
	if !cfg.OracleEnabled {
		return nil
	}

	orc, err := tgateway.CreateOracle(cfg, chainID)
	if err != nil {
		return err
	}

	go orc.RunWithRecovery()
	return nil
}

func initDB(name, dir string) error {
	dbPath := filepath.Join(dir, name+".db")
	if util.FileExists(dbPath) {
		return errors.New("db already exists")
	}

	return nil
}

func destroyDB(name, dir string) error {
	dbPath := filepath.Join(dir, name+".db")
	return os.RemoveAll(dbPath)
}

func resetApp(cfg *config.Config) error {
	return destroyDB(cfg.DBName, cfg.RootPath())
}

func initApp(validator *diadem.Validator, cfg *config.Config) error {
	gen, err := defaultGenesis(cfg, validator)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(cfg.GenesisPath(), os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "    ")
	err = enc.Encode(gen)
	if err != nil {
		return err
	}

	err = initDB(cfg.DBName, cfg.RootPath())
	if err != nil {
		return err
	}

	return nil
}

func destroyApp(cfg *config.Config) error {
	err := util.IgnoreErrNotExists(os.Remove(cfg.GenesisPath()))
	if err != nil {
		return err
	}
	return resetApp(cfg)
}

func destroyReceiptsDB(cfg *config.Config) {
	if cfg.ReceiptsVersion == handler.ReceiptHandlerLevelDb {
		receptHandler := leveldb.LevelDbReceipts{}
		receptHandler.ClearData()
	}
}

func loadAppStore(cfg *config.Config, logger *diadem.Logger, targetVersion int64) (store.VersionedKVStore, error) {
	db, err := cdb.LoadDB(
		cfg.DBBackend, cfg.DBName, cfg.RootPath(), cfg.DBBackendConfig.CacheSizeMegs, cfg.Metrics.Database,
	)
	if err != nil {
		return nil, err
	}

	if cfg.AppStore.CompactOnLoad {
		logger.Info("Compacting app store...")
		if err := db.Compact(); err != nil {
			// compaction erroring out may indicate larger issues with the db,
			// but for now let's try loading the app store anyway...
			logger.Error("Failed to compact app store", "DBName", cfg.DBName, "err", err)
		}
		logger.Info("Finished compacting app store")
	}

	var appStore store.VersionedKVStore
	if cfg.AppStore.Version == 1 { // TODO: cleanup these hardcoded numbers
		if cfg.AppStore.PruneInterval > int64(0) {
			logger.Info("Loading Pruning IAVL Store")
			appStore, err = store.NewPruningIAVLStore(db, store.PruningIAVLStoreConfig{
				MaxVersions: cfg.AppStore.MaxVersions,
				BatchSize:   cfg.AppStore.PruneBatchSize,
				Interval:    time.Duration(cfg.AppStore.PruneInterval) * time.Second,
				Logger:      logger,
			})
			if err != nil {
				return nil, err
			}
		} else {
			logger.Info("Loading IAVL Store")
			appStore, err = store.NewIAVLStore(db, cfg.AppStore.MaxVersions, targetVersion)
			if err != nil {
				return nil, err
			}
		}
	} else if cfg.AppStore.Version == 2 {
		logger.Info("Loading MultiReaderIAVL Store")
		valueDB, err := cdb.LoadDB(
			cfg.AppStore.LatestStateDBBackend, cfg.AppStore.LatestStateDBName, cfg.RootPath(),
			cfg.DBBackendConfig.CacheSizeMegs, cfg.Metrics.Database,
		)
		if err != nil {
			return nil, err
		}
		appStore, err = store.NewMultiReaderIAVLStore(db, valueDB, cfg.AppStore)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("Invalid AppStore.Version config setting")
	}

	if cfg.LogStateDB {
		appStore, err = store.NewLogStore(appStore)
		if err != nil {
			return nil, err
		}
	}

	// NOTE: Shouldn't wrap the MultiReaderIAVLStore in a CachingStore yet, otherwise the
	//       MultiReaderIAVLStore loses its advantages.
	if cfg.CachingStoreConfig.CachingEnabled &&
		((cfg.AppStore.Version == 1) || cfg.CachingStoreConfig.DebugForceEnable) {
		appStore, err = store.NewCachingStore(appStore, cfg.CachingStoreConfig)
		if err != nil {
			return nil, err
		}
		logger.Info("CachingStore enabled")
	}

	return appStore, nil
}

func loadEventStore(cfg *config.Config, logger *diadem.Logger) (store.EventStore, error) {
	eventStoreCfg := cfg.EventStore
	db, err := cdb.LoadDB(
		eventStoreCfg.DBBackend,
		eventStoreCfg.DBName,
		cfg.RootPath(),
		20, //TODO do we want a separate cache config for eventstore?,
		cfg.Metrics.Database,
	)
	if err != nil {
		return nil, err
	}

	eventStore := store.NewKVEventStore(db)
	return eventStore, nil
}

func loadEvmDB(cfg *config.Config, appHeight int64) (dbm.DB, error) {
	evmDBCfg := cfg.EvmDB
	db, err := cdb.LoadDB(
		evmDBCfg.DBBackend,
		evmDBCfg.DBName,
		cfg.RootPath(),
		evmDBCfg.CacheSizeMegs,
		cfg.Metrics.Database,
	)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func loadApp(
	chainID string,
	cfg *config.Config,
	loader plugin.Loader,
	b backend.Backend,
	appHeight int64,
) (*diademchain.Application, error) {
	logger := log.Root

	appStore, err := loadAppStore(cfg, log.Default, appHeight)
	if err != nil {
		return nil, err
	}
	var evmDB dbm.DB
	var eventStore store.EventStore
	var eventDispatcher diademchain.EventDispatcher
	switch cfg.EventDispatcher.Dispatcher {
	case events.DispatcherDBIndexer:
		logger.Info("Using DB indexer event dispatcher")
		eventStore, err = loadEventStore(cfg, log.Default)
		if err != nil {
			return nil, err
		}
		eventDispatcher = events.NewDBIndexerEventDispatcher(eventStore)

	case events.DispatcherRedis:
		uri := cfg.EventDispatcher.Redis.URI
		logger.Info("Using Redis event dispatcher", "uri", uri)
		eventDispatcher, err = events.NewRedisEventDispatcher(uri)
		if err != nil {
			return nil, err
		}
	case events.DispatcherLog:
		logger.Info("Using simple log event dispatcher")
		eventDispatcher = events.NewLogEventDispatcher()
	default:
		return nil, fmt.Errorf("invalid event dispatcher %s", cfg.EventDispatcher.Dispatcher)
	}

	var eventHandler diademchain.EventHandler = diademchain.NewDefaultEventHandler(eventDispatcher)
	if cfg.Metrics.EventHandling {
		eventHandler = diademchain.NewInstrumentingEventHandler(eventHandler)
	}

	// TODO: It shouldn't be possible to change the registry version via config after the first run,
	//       changing it from that point on should require a special upgrade tx that stores the
	//       new version in the app store.
	regVer, err := registry.RegistryVersionFromInt(cfg.RegistryVersion)
	if err != nil {
		return nil, err
	}
	createRegistry, err := registry.NewRegistryFactory(regVer)
	if err != nil {
		return nil, err
	}

	receiptHandlerProvider := receipts.NewReceiptHandlerProvider(eventHandler, func(blockHeight int64, v2Feature bool) (handler.ReceiptHandlerVersion, uint64, error) {
		var receiptVer handler.ReceiptHandlerVersion
		if v2Feature {
			receiptVer = handler.ReceiptHandlerLevelDb
		} else {
			var err error
			receiptVer, err = handler.ReceiptHandlerVersionFromInt(replay.OverrideConfig(cfg, blockHeight).ReceiptsVersion)
			if err != nil {
				return 0, 0, errors.Wrap(err, "failed to resolve receipt handler version")
			}
		}
		return receiptVer, cfg.EVMPersistentTxReceiptsMax, nil
	})

	var newABMFactory plugin.NewAccountBalanceManagerFactoryFunc
	if evm.EVMEnabled && cfg.EVMAccountsEnabled {
		newABMFactory = plugin.NewAccountBalanceManagerFactory
	}

	vmManager := vm.NewManager()
	vmManager.Register(vm.VMType_PLUGIN, func(state diademchain.State) (vm.VM, error) {
		v2ReceiptsEnabled := state.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false)
		receiptReader, err := receiptHandlerProvider.ReaderAt(state.Block().Height, v2ReceiptsEnabled)
		if err != nil {
			return nil, err
		}
		receiptWriter, err := receiptHandlerProvider.WriterAt(state.Block().Height, v2ReceiptsEnabled)
		if err != nil {
			return nil, err
		}
		return plugin.NewPluginVM(
			loader,
			state,
			evmDB,
			createRegistry(state),
			eventHandler,
			log.Default,
			newABMFactory,
			receiptWriter,
			receiptReader,
		), nil
	})

	if evm.EVMEnabled {
		evmDB, err = loadEvmDB(cfg, appHeight)
		if err != nil {
			return nil, err
		}
		vmManager.Register(vm.VMType_EVM, func(state diademchain.State) (vm.VM, error) {
			var createABM evm.AccountBalanceManagerFactoryFunc
			var err error
			v2ReceiptsEnabled := state.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false)
			receiptReader, err := receiptHandlerProvider.ReaderAt(state.Block().Height, v2ReceiptsEnabled)
			if err != nil {
				return nil, err
			}
			receiptWriter, err := receiptHandlerProvider.WriterAt(state.Block().Height, v2ReceiptsEnabled)
			if err != nil {
				return nil, err
			}

			if newABMFactory != nil {
				pvm := plugin.NewPluginVM(
					loader,
					state,
					evmDB,
					createRegistry(state),
					eventHandler,
					log.Default,
					newABMFactory,
					receiptWriter,
					receiptReader,
				)
				createABM, err = newABMFactory(pvm)
				if err != nil {
					return nil, err
				}
			}
			return evm.NewDiademVm(state, evmDB, eventHandler, receiptWriter, createABM, cfg.EVMDebugEnabled), nil
		})
	}
	evm.LogEthDbBatch = cfg.LogEthDbBatch

	deployTxHandler := &vm.DeployTxHandler{
		Manager:        vmManager,
		CreateRegistry: createRegistry,
	}

	callTxHandler := &vm.CallTxHandler{
		Manager: vmManager,
	}

	migrationTxHandler := &tx_handler.MigrationTxHandler{
		Manager:        vmManager,
		CreateRegistry: createRegistry,
		Migrations: map[int32]tx_handler.MigrationFunc{
			1: migrations.DPOSv3Migration,
		},
	}

	gen, err := config.ReadGenesis(cfg.GenesisPath())
	if err != nil {
		return nil, err
	}

	rootAddr := diadem.RootAddress(chainID)
	init := func(state diademchain.State) error {
		registry := createRegistry(state)
		evm.AddDiademPrecompiles()
		for i, contractCfg := range gen.Contracts {
			err := deployContract(
				state,
				contractCfg,
				vmManager,
				rootAddr,
				registry,
				logger,
				i,
			)
			if err != nil {
				return errors.Wrapf(err, "deploying contract: %s", contractCfg.Name)
			}
		}
		return nil
	}

	router := diademchain.NewTxRouter()

	isEvmTx := func(txID uint32, state diademchain.State, txBytes []byte, isCheckTx bool) bool {
		var msg vm.MessageTx
		err := proto.Unmarshal(txBytes, &msg)
		if err != nil {
			return false
		}

		switch txID {
		case 1:
			var tx vm.DeployTx
			err = proto.Unmarshal(msg.Data, &tx)
			if err != nil {
				// In case of error, let's give safest response,
				// let's TxHandler down the line, handle it.
				return false
			}
			return tx.VmType == vm.VMType_EVM
		case 2:
			var tx vm.CallTx
			err = proto.Unmarshal(msg.Data, &tx)
			if err != nil {
				// In case of error, let's give safest response,
				// let's TxHandler down the line, handle it.
				return false
			}
			return tx.VmType == vm.VMType_EVM
		case 3:
			return false
		default:
			return false
		}
	}

	router.HandleDeliverTx(1, diademchain.GeneratePassthroughRouteHandler(deployTxHandler))
	router.HandleDeliverTx(2, diademchain.GeneratePassthroughRouteHandler(callTxHandler))
	router.HandleDeliverTx(3, diademchain.GeneratePassthroughRouteHandler(migrationTxHandler))

	// TODO: Write this in more elegant way
	router.HandleCheckTx(1, diademchain.GenerateConditionalRouteHandler(isEvmTx, diademchain.NoopTxHandler, deployTxHandler))
	router.HandleCheckTx(2, diademchain.GenerateConditionalRouteHandler(isEvmTx, diademchain.NoopTxHandler, callTxHandler))
	router.HandleCheckTx(3, diademchain.GenerateConditionalRouteHandler(isEvmTx, diademchain.NoopTxHandler, migrationTxHandler))

	txMiddleWare := []diademchain.TxMiddleware{
		diademchain.LogTxMiddleware,
		diademchain.RecoveryTxMiddleware,
	}

	txMiddleWare = append(txMiddleWare, auth.NewChainConfigMiddleware(
		cfg.Auth,
		getContractCtx("addressmapper", vmManager),
	))

	createKarmaContractCtx := getContractCtx("karma", vmManager)

	if cfg.Karma.Enabled {
		txMiddleWare = append(txMiddleWare, throttle.GetKarmaMiddleWare(
			cfg.Karma.Enabled,
			cfg.Karma.MaxCallCount,
			cfg.Karma.SessionDuration,
			createKarmaContractCtx,
		))
	}

	if cfg.DeployerWhitelist.ContractEnabled {
		contextFactory := getContractCtx("deployerwhitelist", vmManager)
		dwMiddleware, err := throttle.NewDeployerWhitelistMiddleware(contextFactory)
		if err != nil {
			return nil, err
		}
		txMiddleWare = append(txMiddleWare, dwMiddleware)
	}

	createContractUpkeepHandler := func(state diademchain.State) (diademchain.KarmaHandler, error) {
		// TODO: This setting should be part of the config stored within the Karma contract itself,
		//       that will allow us to switch the upkeep on & off via a tx.
		if !cfg.Karma.UpkeepEnabled {
			return nil, nil
		}
		karmaContractCtx, err := createKarmaContractCtx(state)
		if err != nil {
			// Contract upkeep functionality depends on the Karma contract, so this feature will
			// remain disabled if the Karma contract hasn't been deployed yet.
			if err == regcommon.ErrNotFound {
				return nil, nil
			}
			return nil, err
		}
		return karma_handler.NewKarmaHandler(karmaContractCtx), nil
	}

	getValidatorSet := func(state diademchain.State) (diadem.ValidatorSet, error) {
		if cfg.DPOSVersion == 3 || state.FeatureEnabled(diademchain.DPOSVersion3Feature, false) {
			createDPOSV3Ctx := getContractCtx("dposV3", vmManager)
			dposV3Ctx, err := createDPOSV3Ctx(state)
			if err != nil {
				return nil, err
			}
			validators, err := dposv3.ValidatorList(dposV3Ctx)
			if err != nil {
				return nil, err
			}
			return diadem.NewValidatorSet(validators...), nil
		} else if cfg.DPOSVersion == 2 {
			createDPOSV2Ctx := getContractCtx("dposV2", vmManager)
			dposV2Ctx, err := createDPOSV2Ctx(state)
			if err != nil {
				return nil, err
			}
			validators, err := dposv2.ValidatorList(dposV2Ctx)
			if err != nil {
				return nil, err
			}
			return diadem.NewValidatorSet(validators...), nil
		}

		// if DPOS contract is not deployed, get validators from genesis file
		return diadem.NewValidatorSet(b.GenesisValidators()...), nil
	}

	txMiddleWare = append(txMiddleWare, auth.NonceTxMiddleware)

	oracle, err := diadem.ParseAddress(cfg.Oracle)
	if err != nil {
		oracle = diadem.Address{}
	}
	deployerAddressList, err := cfg.TxLimiter.DeployerAddresses()
	if err != nil {
		return nil, err
	}
	deployerAddressList = append(deployerAddressList, oracle)

	originHandler := throttle.NewOriginValidator(
		uint64(cfg.TxLimiter.CallSessionDuration),
		deployerAddressList,
		cfg.TxLimiter.LimitDeploys,
		cfg.TxLimiter.LimitCalls,
	)

	// Technically ThrottleTxMiddleWare has been replaced by OriginHandler but to replay a couple
	// of old PlasmaChain production blocks correctly we have to keep this middleware around.
	// TODO: Implement height-based middleware overrides so this middleware is only activated for
	//       two blocks in PlasmaChain builds.
	txMiddleWare = append(txMiddleWare, throttle.GetThrottleTxMiddleWare(
		func(blockHeight int64) bool {
			return replay.OverrideConfig(cfg, blockHeight).DeployEnabled
		},
		func(blockHeight int64) bool {
			return replay.OverrideConfig(cfg, blockHeight).CallEnabled
		},
		oracle,
	))

	if cfg.GoContractDeployerWhitelist.Enabled {
		goDeployers, err := cfg.GoContractDeployerWhitelist.DeployerAddresses(chainID)
		if err != nil {
			return nil, errors.Wrapf(err, "getting list of users allowed go deploys")
		}
		txMiddleWare = append(txMiddleWare, throttle.GetGoDeployTxMiddleWare(goDeployers))
	}

	txMiddleWare = append(txMiddleWare, diademchain.NewInstrumentingTxMiddleware())

	createValidatorsManager := func(state diademchain.State) (diademchain.ValidatorsManager, error) {
		pvm, err := vmManager.InitVM(vm.VMType_PLUGIN, state)
		if err != nil {
			return nil, err
		}
		// DPOSv3 can only be enabled via feature flag or if it's enabled via the diadem.yml
		if cfg.DPOSVersion == 3 || state.FeatureEnabled(diademchain.DPOSVersion3Feature, false) {
			return plugin.NewValidatorsManagerV3(pvm.(*plugin.PluginVM))
		} else if cfg.DPOSVersion == 2 {
			return plugin.NewValidatorsManager(pvm.(*plugin.PluginVM))
		}

		return plugin.NewNoopValidatorsManager(), nil
	}

	createChainConfigManager := func(state diademchain.State) (diademchain.ChainConfigManager, error) {
		if !cfg.ChainConfig.ContractEnabled {
			return nil, nil
		}
		pvm, err := vmManager.InitVM(vm.VMType_PLUGIN, state)
		if err != nil {
			return nil, err
		}

		m, err := plugin.NewChainConfigManager(pvm.(*plugin.PluginVM), state)
		if err != nil {
			// This feature will remain disabled until the ChainConfig contract is deployed
			if err == plugin.ErrChainConfigContractNotFound {
				return nil, nil
			}
			return nil, err
		}
		return m, nil
	}

	postCommitMiddlewares := []diademchain.PostCommitMiddleware{
		diademchain.LogPostCommitMiddleware,
		auth.NonceTxPostNonceMiddleware,
	}
	if !cfg.Karma.Enabled && cfg.Karma.UpkeepEnabled {
		logger.Info("Karma disabled, upkeep enabled ignored")
	}

	return &diademchain.Application{
		Store: appStore,
		Init:  init,
		TxHandler: diademchain.MiddlewareTxHandler(
			txMiddleWare,
			router,
			postCommitMiddlewares,
		),
		EventHandler:                eventHandler,
		ReceiptHandlerProvider:      receiptHandlerProvider,
		CreateValidatorManager:      createValidatorsManager,
		CreateChainConfigManager:    createChainConfigManager,
		CreateContractUpkeepHandler: createContractUpkeepHandler,
		OriginHandler:               &originHandler,
		EventStore:                  eventStore,
		EvmDB:                       evmDB,
		GetValidatorSet:             getValidatorSet,
	}, nil
}

func deployContract(
	state diademchain.State,
	contractCfg config.ContractConfig,
	vmManager *vm.Manager,
	rootAddr diadem.Address,
	registry regcommon.Registry,
	logger log.TMLogger,
	index int,
) error {
	vmType := contractCfg.VMType()
	vm, err := vmManager.InitVM(vmType, state)
	if err != nil {
		return err
	}

	loader := codeLoaders[contractCfg.Format]
	initCode, err := loader.LoadContractCode(
		contractCfg.Location,
		contractCfg.Init,
	)
	if err != nil {
		return err
	}

	callerAddr := plugin.CreateAddress(rootAddr, uint64(index))
	_, addr, err := vm.Create(callerAddr, initCode, diadem.NewBigUIntFromInt(0))
	if err != nil {
		return err
	}

	err = registry.Register(contractCfg.Name, addr, addr)
	if err != nil {
		return err
	}

	logger.Info("Deployed contract",
		"vm", contractCfg.VMTypeName,
		"location", contractCfg.Location,
		"name", contractCfg.Name,
		"address", addr,
	)
	return nil
}

type contextFactory func(state diademchain.State) (contractpb.Context, error)

func getContractCtx(pluginName string, vmManager *vm.Manager) contextFactory {
	return func(state diademchain.State) (contractpb.Context, error) {
		pvm, err := vmManager.InitVM(vm.VMType_PLUGIN, state)
		if err != nil {
			return nil, err
		}
		return plugin.NewInternalContractContext(pluginName, pvm.(*plugin.PluginVM))
	}
}

func initBackend(cfg *config.Config, abciServerAddr string, fnRegistry fnConsensus.FnRegistry) backend.Backend {
	ovCfg := &backend.OverrideConfig{
		LogLevel:                 cfg.BlockchainLogLevel,
		Peers:                    cfg.Peers,
		PersistentPeers:          cfg.PersistentPeers,
		ChainID:                  cfg.ChainID,
		RPCListenAddress:         cfg.RPCListenAddress,
		RPCProxyPort:             cfg.RPCProxyPort,
		CreateEmptyBlocks:        cfg.CreateEmptyBlocks,
		HsmConfig:                cfg.HsmConfig,
		FnConsensusReactorConfig: cfg.FnConsensus.Reactor,
	}
	return &backend.TendermintBackend{
		RootPath:    path.Join(cfg.RootPath(), "chaindata"),
		OverrideCfg: ovCfg,
		SocketPath:  abciServerAddr,
		FnRegistry:  fnRegistry,
	}
}

func initQueryService(
	app *diademchain.Application, chainID string, cfg *config.Config, loader plugin.Loader,
	receiptHandlerProvider diademchain.ReceiptHandlerProvider,
) error {
	// metrics
	fieldKeys := []string{"method", "error"}
	requestCount := kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: "diademchain",
		Subsystem: "query_service",
		Name:      "request_count",
		Help:      "Number of requests received.",
	}, fieldKeys)
	requestLatency := kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace: "diademchain",
		Subsystem: "query_service",
		Name:      "request_latency_microseconds",
		Help:      "Total duration of requests in microseconds.",
	}, fieldKeys)

	regVer, err := registry.RegistryVersionFromInt(cfg.RegistryVersion)
	if err != nil {
		return err
	}
	createRegistry, err := registry.NewRegistryFactory(regVer)
	if err != nil {
		return err
	}

	var newABMFactory plugin.NewAccountBalanceManagerFactoryFunc
	if evm.EVMEnabled && cfg.EVMAccountsEnabled {
		newABMFactory = plugin.NewAccountBalanceManagerFactory
	}

	blockstore, err := store.NewBlockStore(cfg.BlockStore)
	if err != nil {
		return err
	}

	qs := &rpc.QueryServer{
		StateProvider:          app,
		ChainID:                chainID,
		Loader:                 loader,
		Subscriptions:          app.EventHandler.SubscriptionSet(),
		EthSubscriptions:       app.EventHandler.EthSubscriptionSet(),
		EthLegacySubscriptions: app.EventHandler.LegacyEthSubscriptionSet(),
		EthPolls:               *polls.NewEthSubscriptions(),
		CreateRegistry:         createRegistry,
		NewABMFactory:          newABMFactory,
		ReceiptHandlerProvider: receiptHandlerProvider,
		RPCListenAddress:       cfg.RPCListenAddress,
		BlockStore:             blockstore,
		EventStore:             app.EventStore,
		EvmDB:                  app.EvmDB,
		AuthCfg:                cfg.Auth,
	}
	bus := &rpc.QueryEventBus{
		Subs:    *app.EventHandler.SubscriptionSet(),
		EthSubs: *app.EventHandler.LegacyEthSubscriptionSet(),
	}
	// query service
	var qsvc rpc.QueryService
	{
		qsvc = qs
		qsvc = rpc.NewInstrumentingMiddleWare(requestCount, requestLatency, qsvc)
	}
	logger := log.Root.With("module", "query-server")
	err = rpc.RPCServer(qsvc, logger, bus, cfg.RPCBindAddress, cfg.UnsafeRPCEnabled, cfg.UnsafeRPCBindAddress)
	if err != nil {
		return err
	}

	return nil
}

func startPushGatewayMonitoring(cfg *config.PrometheusPushGatewayConfig, log *diadem.Logger, host string) {
	for {
		time.Sleep(time.Duration(cfg.PushRateInSeconds) * time.Second)
		err := push.New(cfg.PushGateWayUrl, cfg.JobName).Grouping("instance", host).Gatherer(prometheus.DefaultGatherer).Push()
		if err != nil {
			log.Error("Error in pushing to Prometheus Push Gateway ", "Error", err)
		}
	}
}

func main() {
	karmaCmd := cli.ContractCallCommand(KarmaContractName)
	addressMappingCmd := cli.ContractCallCommand(AddressMapperName)
	callCommand := cli.ContractCallCommand("")
	resolveCmd := cli.ContractCallCommand("resolve")
	commands.AddGeneralCommands(resolveCmd)

	unsafeCmd := cli.ContractCallCommand("unsafe")
	commands.AddUnsafeCommands(unsafeCmd)

	commands.Add(callCommand)
	RootCmd.AddCommand(
		newVersionCommand(),
		newEnvCommand(),
		newInitCommand(),
		newResetCommand(),
		newRunCommand(),
		newSpinCommand(),
		newDeployCommand(),
		newDeployGoCommand(),
		newMigrationCommand(),
		callCommand,
		newGenKeyCommand(),
		newYubiHsmCommand(),
		newNodeKeyCommand(),
		newStaticCallCommand(), //Depreciate
		newGetBlocksByNumber(),
		NewCoinCommand(),
		NewDPOSV2Command(),
		NewDPOSV3Command(),
		karmaCmd,
		addressMappingCmd,
		gatewaycmd.NewGatewayCommand(),
		dbcmd.NewDBCommand(),
		newCallEvmCommand(), //Depreciate
		resolveCmd,
		unsafeCmd,
		commands.GetMapping(),
		commands.ListMapping(),
		staking.NewStakingCommand(),
		chaincfgcmd.NewChainCfgCommand(),
		deployer.NewDeployCommand(),
		dbg.NewDebugCommand(),
	)
	AddKarmaMethods(karmaCmd)
	AddAddressMappingMethods(addressMappingCmd)
	err := RootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
