package chainreader

import (
	"context"
	"encoding/json"

	ag_solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

const ServiceName = "SolanaChainReader"

type SolanaChainReaderService struct {
	// provided values
	lggr   logger.Logger
	client BinaryDataReader

	// internal values
	bindings namespaceBindings

	// service state management
	services.StateMachine
}

var (
	_ services.Service  = &SolanaChainReaderService{}
	_ types.ChainReader = &SolanaChainReaderService{}
)

// NewChainReaderService is a constructor for a new ChainReaderService for Solana. Returns a nil service on error.
func NewChainReaderService(lggr logger.Logger, dataReader BinaryDataReader, cfg config.ChainReader) (*SolanaChainReaderService, error) {
	svc := &SolanaChainReaderService{
		lggr:     logger.Named(lggr, ServiceName),
		client:   dataReader,
		bindings: namespaceBindings{},
	}

	if err := svc.init(cfg.Namespaces); err != nil {
		return nil, err
	}

	return svc, nil
}

// Name implements the services.ServiceCtx interface and returns the logger service name.
func (s *SolanaChainReaderService) Name() string {
	return s.lggr.Name()
}

// Start implements the services.ServiceCtx interface and starts necessary background services.
// An error is returned if starting any internal services fails. Subsequent calls to Start return
// and error.
func (s *SolanaChainReaderService) Start(_ context.Context) error {
	return s.StartOnce(ServiceName, func() error {
		return nil
	})
}

// Close implements the services.ServiceCtx interface and stops all background services and cleans
// up used resources. Subsequent calls to Close return an error.
func (s *SolanaChainReaderService) Close() error {
	return s.StopOnce(ServiceName, func() error {
		return nil
	})
}

// Ready implements the services.ServiceCtx interface and returns an error if starting the service
// encountered any errors or if the service is not ready to serve requests.
func (s *SolanaChainReaderService) Ready() error {
	return s.StateMachine.Ready()
}

// HealthReport implements the services.ServiceCtx interface and returns errors for any internal
// function or service that may have failed.
func (s *SolanaChainReaderService) HealthReport() map[string]error {
	return map[string]error{s.Name(): s.Healthy()}
}

// GetLatestValue implements the types.ChainReader interface and requests and parses on-chain
// data named by the provided contract, method, and params.
func (s *SolanaChainReaderService) GetLatestValue(ctx context.Context, contractName, method string, params any, returnVal any) error {
	bindings, err := s.bindings.GetReadBindings(contractName, method)
	if err != nil {
		return err
	}

	for _, binding := range bindings {
		if err := binding.GetLatestValue(ctx, params, returnVal); err != nil {
			return err
		}
	}

	return nil
}

// Bind implements the types.ChainReader interface and allows new contract bindings to be added
// to the service.
func (s *SolanaChainReaderService) Bind(_ context.Context, bindings []types.BoundContract) error {
	return s.bindings.Bind(bindings)
}

// CreateContractType implements the ContractTypeProvider interface and allows the chain reader
// service to explicitly define the expected type for a grpc server to provide.
func (s *SolanaChainReaderService) CreateContractType(contractName, itemType string, forEncoding bool) (any, error) {
	return s.bindings.CreateType(contractName, itemType, forEncoding)
}

func (s *SolanaChainReaderService) init(namespaces map[string]config.ChainReaderMethods) error {
	for namespace, methods := range namespaces {
		for methodName, method := range methods.Methods {
			var idl codec.IDL
			if err := json.Unmarshal([]byte(method.AnchorIDL), &idl); err != nil {
				return err
			}

			idlCodec, err := codec.NewIDLCodec(idl)
			if err != nil {
				return err
			}

			for _, procedure := range method.Procedures {
				mod, err := procedure.OutputModifications.ToModifier(codec.DecoderHooks...)
				if err != nil {
					return err
				}

				codecWithModifiers, err := codec.NewNamedModifierCodec(idlCodec, procedure.IDLAccount, mod)
				if err != nil {
					return err
				}

				s.bindings.AddReadBinding(namespace, methodName, &accountReadBinding{
					idlAccount: procedure.IDLAccount,
					codec:      codecWithModifiers,
					reader:     s.client,
				})
			}
		}
	}

	return nil
}

type accountDataReader struct {
	client *rpc.Client
}

func NewAccountDataReader(client *rpc.Client) *accountDataReader {
	return &accountDataReader{client: client}
}

func (r *accountDataReader) ReadAll(ctx context.Context, pk ag_solana.PublicKey) ([]byte, error) {
	result, err := r.client.GetAccountInfo(ctx, pk)
	if err != nil {
		return nil, err
	}

	bts := result.Value.Data.GetBinary()

	return bts, nil
}
