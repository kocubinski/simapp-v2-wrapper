package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	cmtcfg "github.com/cometbft/cometbft/config"
	"github.com/spf13/cobra"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	"cosmossdk.io/client/v2/autocli"
	"cosmossdk.io/core/transaction"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	"cosmossdk.io/runtime/v2"
	serverv2 "cosmossdk.io/server/v2"
	"cosmossdk.io/server/v2/cometbft"
	"cosmossdk.io/simapp/v2"
	"cosmossdk.io/simapp/v2/simdv2/cmd"

	"github.com/cosmos/cosmos-sdk/client"
	nodeservice "github.com/cosmos/cosmos-sdk/client/grpc/node"
)

func main() {
	args := os.Args[1:]
	rootCmd, err := newRootCmd[transaction.Tx](args...)
	if err != nil {
		if _, pErr := fmt.Fprintln(os.Stderr, err); pErr != nil {
			panic(errors.Join(err, pErr))
		}
		os.Exit(1)
	}
	if err = rootCmd.Execute(); err != nil {
		if _, pErr := fmt.Fprintln(rootCmd.OutOrStderr(), err); pErr != nil {
			panic(errors.Join(err, pErr))
		}
		os.Exit(1)
	}
}

func newRootCmd[T transaction.Tx](
	args ...string,
) (*cobra.Command, error) {
	rootCommand := &cobra.Command{
		Use:           "notsimd",
		SilenceErrors: true,
	}

	// TODO
	// here I've initialized the consensus component as cometbft, but it could be any other consensus
	// engine satisfying the serverv2.ServerComponent interface
	var consensus serverv2.ServerComponent[T] = cometbft.NewWithConfigOptions[T](initCometConfig())
	configWriter, err := cmd.InitRootCmd(rootCommand, log.NewNopLogger(), cmd.CommandDependencies[T]{
		Consensus: consensus,
	})
	if err != nil {
		return nil, err
	}
	factory, err := serverv2.NewCommandFactory(
		serverv2.WithConfigWriter(configWriter),
		serverv2.WithStdDefaultHomeDir(".notsimd"),
		serverv2.WithLoggerFactory(serverv2.NewLogger),
	)
	if err != nil {
		return nil, err
	}

	subCommand, configMap, logger, err := factory.ParseCommand(rootCommand, args)
	if err != nil {
		return nil, err
	}

	var (
		autoCliOpts     autocli.AppOptions
		moduleManager   *runtime.MM[T]
		clientCtx       client.Context
		simApp          *simapp.SimApp[T]
		depinjectConfig = depinject.Configs(
			depinject.Supply(logger, runtime.GlobalConfig(configMap)),
			depinject.Provide(cmd.ProvideClientContext),
		)
	)
	if serverv2.IsAppRequired(subCommand) {
		// server construction
		simApp, err = simapp.NewSimApp[T](depinjectConfig, &autoCliOpts, &moduleManager, &clientCtx)
		if err != nil {
			return nil, err
		}
		// TODO replace with a custom consensus engine
		consensus, err = cometbft.New(
			logger,
			simApp.Name(),
			simApp.Store(),
			simApp.App.AppManager,
			simApp.App.QueryHandlers(),
			simApp.App.SchemaDecoderResolver(),
			&genericTxDecoder[T]{clientCtx.TxConfig},
			configMap,
			cometbft.DefaultServerOptions[T](),
		)
		if err != nil {
			return nil, err
		}
	} else {
		// client construction
		if err = depinject.Inject(
			depinject.Configs(
				simapp.AppConfig(),
				depinjectConfig,
			),
			&autoCliOpts, &moduleManager, &clientCtx,
		); err != nil {
			return nil, err
		}
	}

	commandDeps := cmd.CommandDependencies[T]{
		GlobalConfig:  configMap,
		TxConfig:      clientCtx.TxConfig,
		ModuleManager: moduleManager,
		SimApp:        simApp,
		Consensus:     consensus,
	}
	rootCommand = &cobra.Command{
		Use:               "notsimd",
		Short:             "not a simulation app",
		SilenceErrors:     true,
		PersistentPreRunE: cmd.RootCommandPersistentPreRun(clientCtx),
	}
	factory.EnhanceRootCommand(rootCommand)
	_, err = cmd.InitRootCmd(rootCommand, logger, commandDeps)
	if err != nil {
		return nil, err
	}
	nodeCmds := nodeservice.NewNodeCommands()
	autoCliOpts.ModuleOptions = make(map[string]*autocliv1.ModuleOptions)
	autoCliOpts.ModuleOptions[nodeCmds.Name()] = nodeCmds.AutoCLIOptions()
	if err := autoCliOpts.EnhanceRootCommand(rootCommand); err != nil {
		return nil, err
	}

	return rootCommand, nil
}

// TODO Delete below this line.
// Only required for cometbft consensus engine.

func initCometConfig() cometbft.CfgOption {
	cfg := cmtcfg.DefaultConfig()

	// display only warn logs by default except for p2p and state
	cfg.LogLevel = "*:warn,server:info,p2p:info,state:info"
	// increase block timeout
	cfg.Consensus.TimeoutCommit = 5 * time.Second
	// overwrite default pprof listen address
	cfg.RPC.PprofListenAddress = "localhost:6060"

	return cometbft.OverwriteDefaultConfigTomlConfig(cfg)
}

var _ transaction.Codec[transaction.Tx] = &genericTxDecoder[transaction.Tx]{}

type genericTxDecoder[T transaction.Tx] struct {
	txConfig client.TxConfig
}

// Decode implements transaction.Codec.
func (t *genericTxDecoder[T]) Decode(bz []byte) (T, error) {
	var out T
	tx, err := t.txConfig.TxDecoder()(bz)
	if err != nil {
		return out, err
	}

	var ok bool
	out, ok = tx.(T)
	if !ok {
		return out, errors.New("unexpected Tx type")
	}

	return out, nil
}

// DecodeJSON implements transaction.Codec.
func (t *genericTxDecoder[T]) DecodeJSON(bz []byte) (T, error) {
	var out T
	tx, err := t.txConfig.TxJSONDecoder()(bz)
	if err != nil {
		return out, err
	}

	var ok bool
	out, ok = tx.(T)
	if !ok {
		return out, errors.New("unexpected Tx type")
	}

	return out, nil
}
