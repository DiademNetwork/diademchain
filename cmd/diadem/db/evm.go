// +build evm

package db

import (
	"context"
	"fmt"

	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/cmd/diadem/common"
	"github.com/diademnetwork/diademchain/cmd/diadem/replay"
	"github.com/diademnetwork/diademchain/events"
	"github.com/diademnetwork/diademchain/evm"
	"github.com/diademnetwork/diademchain/log"
	"github.com/diademnetwork/diademchain/plugin"
	"github.com/diademnetwork/diademchain/receipts"
	"github.com/diademnetwork/diademchain/receipts/handler"
	registry "github.com/diademnetwork/diademchain/registry/factory"
	"github.com/diademnetwork/diademchain/store"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	abci "github.com/tendermint/tendermint/abci/types"
	dbm "github.com/tendermint/tendermint/libs/db"
)

func newDumpEVMStateCommand() *cobra.Command {
	var appHeight int64
	var evmDBFlag bool

	cmd := &cobra.Command{
		Use:   "evm-dump",
		Short: "Dumps EVM state stored at a specific block height",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := common.ParseConfig()
			if err != nil {
				return err
			}

			db, err := dbm.NewGoLevelDB(cfg.DBName, cfg.RootPath())
			if err != nil {
				return err
			}
			appStore, err := store.NewIAVLStore(db, 0, appHeight)
			if err != nil {
				return err
			}

			eventHandler := diademchain.NewDefaultEventHandler(events.NewLogEventDispatcher())

			regVer, err := registry.RegistryVersionFromInt(cfg.RegistryVersion)
			if err != nil {
				return err
			}
			createRegistry, err := registry.NewRegistryFactory(regVer)
			if err != nil {
				return err
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

			// TODO: This should use snapshot obtained from appStore.ReadOnlyState()
			storeTx := store.WrapAtomic(appStore).BeginTx()
			evmDB, err := dbm.NewGoLevelDB(cfg.EvmDB.DBName, cfg.RootPath())
			if err != nil {
				return err
			}

			state := diademchain.NewStoreState(
				context.Background(),
				storeTx,
				abci.Header{
					Height: appStore.Version(),
				},
				// it is possible to load the block hash from the TM block store, but probably don't
				// need it for just dumping the EVM state
				nil,
				nil,
			)

			receiptReader, err := receiptHandlerProvider.ReaderAt(state.Block().Height, state.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
			if err != nil {
				return err
			}
			receiptWriter, err := receiptHandlerProvider.WriterAt(state.Block().Height, state.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
			if err != nil {
				return err
			}

			var newABMFactory plugin.NewAccountBalanceManagerFactoryFunc
			if evm.EVMEnabled && cfg.EVMAccountsEnabled {
				newABMFactory = plugin.NewAccountBalanceManagerFactory
			}

			var accountBalanceManager evm.AccountBalanceManager
			if newABMFactory != nil {
				pvm := plugin.NewPluginVM(
					common.NewDefaultContractsLoader(cfg),
					state,
					evmDB,
					createRegistry(state),
					eventHandler,
					log.Default,
					newABMFactory,
					receiptWriter,
					receiptReader,
				)
				createABM, err := newABMFactory(pvm)
				if err != nil {
					return err
				}
				accountBalanceManager = createABM(true)
				if err != nil {
					return err
				}
			}

			state.SetFeature(diademchain.EvmDBFeature, evmDBFlag)

			vm, err := evm.NewDiademEvm(state, evmDB, accountBalanceManager, nil, false)
			if err != nil {
				return err
			}

			fmt.Printf("\n--- EVM state at app height %d ---\n%s\n", appStore.Version(), string(vm.RawDump()))
			return nil
		},
	}

	cmdFlags := cmd.Flags()
	cmdFlags.Int64Var(&appHeight, "app-height", 0, "Dump EVM state as it was the specified app height")
	cmdFlags.BoolVar(&evmDBFlag, "evmdb", false, "Dump EVM state from evm.db instead of app.db")
	return cmd
}

func newMigrateEvmStateCommand() *cobra.Command {
	var appHeight int64
	vmPrefix := []byte("vm")
	cmd := &cobra.Command{
		Use:   "evm-migrate",
		Short: "Migrate EVM state stored in app.db at a specific block height to evm.db",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := common.ParseConfig()
			if err != nil {
				return err
			}

			appDB, err := dbm.NewGoLevelDB(cfg.DBName, cfg.RootPath())
			if err != nil {
				return err
			}

			evmDB, err := dbm.NewGoLevelDB(cfg.EvmDB.DBName, cfg.RootPath())
			if err != nil {
				return err
			}

			appStore, err := store.NewIAVLStore(appDB, 0, appHeight)
			if err != nil {
				return err
			}

			for _, d := range appStore.Range(vmPrefix) {
				evmDB.Set(d.Key, d.Value)
			}

			return nil
		},
	}

	cmdFlags := cmd.Flags()
	cmdFlags.Int64Var(&appHeight, "app-height", 0, "App height at which EVM state will be migrated from app.db to evm.db")
	return cmd
}
