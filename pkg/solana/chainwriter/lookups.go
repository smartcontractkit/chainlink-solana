package chainwriter

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	types "github.com/smartcontractkit/chainlink-common/pkg/types/solana"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

var (
	ErrLookupNotFoundAtLocation = fmt.Errorf("error getting account from lookup")
	ErrLookupTableNotFound      = fmt.Errorf("lookup table not found")
	ErrGettingSeedAtLocation    = fmt.Errorf("error getting address seed for location")
)

var _ = Lookup(types.Lookup{})

// Lookup is not an alias like the others, because it has a widely used method with solana types that cannot be imported
// in to chainlink-common.  However, it can be directly converted from [types.Lookup].
type Lookup struct {
	Optional                bool
	AccountConstant         *AccountConstant         `json:"accountConstant,omitempty"`
	AccountLookup           *AccountLookup           `json:"accountLookup,omitempty"`
	PDALookups              *PDALookups              `json:"pdas,omitempty"`
	AccountsFromLookupTable *AccountsFromLookupTable `json:"accountsFromLookupTable,omitempty"`
}

// Deprecated
type AccountConstant = types.AccountConstant

// Deprecated
type AccountLookup = types.AccountLookup

// Deprecated
type MetaBool = types.MetaBool

// Deprecated
type Seed = types.Seed

// Deprecated
type PDALookups = types.PDALookups

// Deprecated
type InternalField = types.InternalField

// Deprecated
type LookupTables = types.LookupTables

// Deprecated
type DerivedLookupTable = types.DerivedLookupTable

// Deprecated
type AccountsFromLookupTable = types.AccountsFromLookupTable

// Deprecated
type ATALookup = types.ATALookup

func (l Lookup) validate() error {
	count := 0
	for _, v := range []bool{l.AccountConstant != nil, l.AccountLookup != nil, l.PDALookups != nil, l.AccountsFromLookupTable != nil} {
		if v {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("exactly one of AccountConstant, AccountLookup, PDALookups, or AccountsFromLookupTable must be specified, got %d", count)
	}
	return nil
}

func (l Lookup) Resolve(ctx context.Context, args any, derivedTableMap map[string]map[string][]*solana.AccountMeta, client client.MultiClient) ([]*solana.AccountMeta, error) {
	// could update this in the future to validate the entire config at initialization time recursively.
	err := l.validate()
	if err != nil {
		return nil, err
	}
	if l.AccountConstant != nil {
		return ResolveAccountConstant(l.AccountConstant)
	} else if l.AccountLookup != nil {
		return ResolveAccountLookup(l.AccountLookup, args)
	} else if l.PDALookups != nil {
		return ResolvePDALookups(ctx, l.PDALookups, args, derivedTableMap, client)
	} else if l.AccountsFromLookupTable != nil {
		return ResolveAccountsFromLookupTable(l.AccountsFromLookupTable, derivedTableMap)
	}
	return nil, fmt.Errorf("no lookup type specified")
}

func ResolveAccountConstant(ac *AccountConstant) ([]*solana.AccountMeta, error) {
	address, err := solana.PublicKeyFromBase58(ac.Address)
	if err != nil {
		return nil, lookupErrWithName(ac.Name, fmt.Errorf("error getting account from constant: %w", err))
	}
	return []*solana.AccountMeta{
		{
			PublicKey:  address,
			IsSigner:   ac.IsSigner,
			IsWritable: ac.IsWritable,
		},
	}, nil
}

func ResolveAccountLookup(al *AccountLookup, args any) ([]*solana.AccountMeta, error) {
	derivedValues, err := GetValuesAtLocation(args, al.Location)
	if err != nil {
		return nil, lookupErrWithName(al.Name, fmt.Errorf("%w: %v", ErrLookupNotFoundAtLocation, err))
	}

	if len(derivedValues) == 0 {
		// early return, there's nothing to set
		return nil, nil
	}

	var metas []*solana.AccountMeta
	signerIndexes, err := resolveBitMap(al.IsSigner, args, len(derivedValues))
	if err != nil {
		return nil, lookupErrWithName(al.Name, fmt.Errorf("failed to resolve signer bit map: %w", err))
	}

	writerIndexes, err := resolveBitMap(al.IsWritable, args, len(derivedValues))
	if err != nil {
		return nil, lookupErrWithName(al.Name, fmt.Errorf("failed to resolve writer bit map: %w", err))
	}

	for i, address := range derivedValues {
		// Resolve isSigner for this particular pubkey
		isSigner := signerIndexes[i]

		// Resolve isWritable for this particular pubkey
		isWritable := writerIndexes[i]

		metas = append(metas, &solana.AccountMeta{
			PublicKey:  solana.PublicKeyFromBytes(address),
			IsSigner:   isSigner,
			IsWritable: isWritable,
		})
	}
	return metas, nil
}

func resolveBitMap(mb MetaBool, args any, length int) ([]bool, error) {
	if length > 64 {
		return []bool{}, fmt.Errorf("bitmap cannot have more than 64 flags. provided length: %d", length)
	}
	result := make([]bool, length)
	if mb.BitmapLocation == "" {
		for i := 0; i < length; i++ {
			result[i] = mb.Value
		}
		return result, nil
	}

	bitmapVals, err := GetValuesAtLocation(args, mb.BitmapLocation)
	if err != nil {
		return []bool{}, fmt.Errorf("error reading bitmap from location '%s': %w", mb.BitmapLocation, err)
	}

	if len(bitmapVals) != 1 {
		return []bool{}, fmt.Errorf("bitmap value is not a single value: %v, length: %d", bitmapVals, len(bitmapVals))
	}

	if len(bitmapVals[0]) != 8 {
		return []bool{}, fmt.Errorf("bitmap value has insufficient bytes: %v, length: %d", bitmapVals[0], len(bitmapVals[0]))
	}

	// The bitmap is user defined and always uint64 size. The input cannot be further validated for correctness.
	bitmapInt := binary.LittleEndian.Uint64(bitmapVals[0])
	for i := 0; i < length; i++ {
		result[i] = bitmapInt&(1<<i) > 0
	}

	return result, nil
}

func ResolveAccountsFromLookupTable(alt *AccountsFromLookupTable, derivedTableMap map[string]map[string][]*solana.AccountMeta) ([]*solana.AccountMeta, error) {
	// Fetch the inner map for the specified lookup table name
	innerMap, ok := derivedTableMap[alt.LookupTableName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrLookupTableNotFound, alt.LookupTableName)
	}

	var result []*solana.AccountMeta

	// If no indices are specified, include all addresses
	if len(alt.IncludeIndexes) == 0 {
		for _, metas := range innerMap {
			result = append(result, metas...)
		}
		return result, nil
	}

	// Otherwise, include only addresses at the specified indices
	for publicKey, metas := range innerMap {
		for _, index := range alt.IncludeIndexes {
			if index < 0 || index >= len(metas) {
				return nil, lookupErrWithName(alt.LookupTableName, fmt.Errorf("invalid index %d for account %s", index, publicKey))
			}
			result = append(result, metas[index])
		}
	}

	return result, nil
}

func ResolvePDALookups(ctx context.Context, pda *PDALookups, args any, derivedTableMap map[string]map[string][]*solana.AccountMeta, client client.MultiClient) ([]*solana.AccountMeta, error) {
	publicKeys, err := GetAddresses(ctx, args, []types.Lookup{pda.PublicKey}, derivedTableMap, client)
	if err != nil {
		return nil, lookupErrWithName(pda.Name, fmt.Errorf("error getting public key for PDALookups: %w", err))
	}

	seeds, err := getSeedBytesCombinations(ctx, pda, args, derivedTableMap, client)
	if err != nil {
		return nil, lookupErrWithName(pda.Name, fmt.Errorf("error getting seeds for PDALookups: %w", err))
	}

	pdas, err := generatePDAs(publicKeys, seeds, pda)
	if err != nil {
		return nil, lookupErrWithName(pda.Name, fmt.Errorf("error generating PDAs: %w", err))
	}

	if pda.InternalField.Location == "" {
		return pdas, nil
	}

	// If a decoded location is specified, fetch the data at that location
	var result []*solana.AccountMeta
	for _, accountMeta := range pdas {
		accountInfo, err := client.GetAccountInfoWithOpts(ctx, accountMeta.PublicKey, &rpc.GetAccountInfoOpts{
			Encoding:   "base64",
			Commitment: rpc.CommitmentFinalized,
		})

		if err != nil || accountInfo == nil || accountInfo.Value == nil || accountInfo.Value.Data == nil {
			return nil, lookupErrWithName(pda.Name, fmt.Errorf("error fetching account info for PDA account: %s, error: %w", accountMeta.PublicKey.String(), err))
		}

		var idlCodec codec.IDL
		if err = json.Unmarshal([]byte(pda.InternalField.IDL), &idlCodec); err != nil {
			return nil, lookupErrWithName(pda.Name, fmt.Errorf("failed to unmarshal IDL: %w", err))
		}

		internalType := pda.InternalField.TypeName

		idlDef, err := codec.FindDefinitionFromIDL(codec.ChainConfigTypeAccountDef, internalType, idlCodec)
		if err != nil {
			return nil, lookupErrWithName(pda.Name, fmt.Errorf("error finding definition for type %s: %w", internalType, err))
		}

		input, err := codec.CreateCodecEntry(idlDef, internalType, idlCodec, nil)
		if err != nil {
			return nil, lookupErrWithName(pda.Name, fmt.Errorf("failed to create codec entry for type %s, error: %w", internalType, err))
		}

		decoded, _, err := input.Decode(accountInfo.Value.Data.GetBinary())
		if err != nil {
			return nil, lookupErrWithName(pda.Name, fmt.Errorf("error decoding account data: %w", err))
		}

		value, err := GetValuesAtLocation(decoded, pda.InternalField.Location)
		if err != nil {
			return nil, lookupErrWithName(pda.Name, fmt.Errorf("error getting value at location %s: %w", pda.InternalField.Location, err))
		}
		if len(value) > 1 {
			return nil, lookupErrWithName(pda.Name, fmt.Errorf("multiple values found at location %s", pda.InternalField.Location))
		}

		result = append(result, &solana.AccountMeta{
			PublicKey:  solana.PublicKeyFromBytes(value[0]),
			IsSigner:   accountMeta.IsSigner,
			IsWritable: accountMeta.IsWritable,
		})
	}
	return result, nil
}

// getSeedBytesCombinations extracts the seeds for the PDALookups.
// The return type is [][][]byte, where each element of the outer slice is
// one combination of seeds. This handles the case where one seed can resolve
// to multiple addresses, multiplying the combinations accordingly.
func getSeedBytesCombinations(
	ctx context.Context,
	lookup *PDALookups,
	args any,
	derivedTableMap map[string]map[string][]*solana.AccountMeta,
	client client.MultiClient,
) ([][][]byte, error) {
	allCombinations := [][][]byte{
		{},
	}

	// For each seed in the definition, expand the current list of combinations
	// by all possible values for this seed.
	for _, seed := range lookup.Seeds {
		expansions := make([][]byte, 0)
		if seed.Static != nil {
			expansions = append(expansions, seed.Static)
			// Static and Dynamic seeds are mutually exclusive
		} else if !seed.Dynamic.IsNil() {
			dynamicSeed := seed.Dynamic
			if dynamicSeed.AccountLookup != nil {
				lookupSeed := dynamicSeed.AccountLookup
				// Get value from a location (This doesn't have to be an address, it can be any value)
				bytes, err := GetValuesAtLocation(args, lookupSeed.Location)
				if err != nil {
					return nil, fmt.Errorf("%w %q: %w", ErrGettingSeedAtLocation, lookupSeed.Location, err)
				}
				// append each byte array to the expansions
				for _, b := range bytes {
					// validate seed length
					if len(b) > solana.MaxSeedLength {
						return nil, fmt.Errorf("seed byte array exceeds maximum length of %d: got %d bytes", solana.MaxSeedLength, len(b))
					}
					expansions = append(expansions, b)
				}
			} else {
				// Get address seeds from the lookup
				seedAddresses, err := GetAddresses(ctx, args, []types.Lookup{dynamicSeed}, derivedTableMap, client)
				if err != nil {
					return nil, fmt.Errorf("error getting address seed: %w", err)
				}
				// Add each address seed to the expansions
				for _, addrMeta := range seedAddresses {
					b := addrMeta.PublicKey.Bytes()
					if len(b) > solana.MaxSeedLength {
						return nil, fmt.Errorf("seed byte array exceeds maximum length of %d: got %d bytes", solana.MaxSeedLength, len(b))
					}
					expansions = append(expansions, b)
				}
			}
		}

		// expansions is the list of possible seed bytes for this single seed lookup.
		// Multiply the existing combinations in allCombinations by each item in expansions.
		newCombinations := make([][][]byte, 0, len(allCombinations)*len(expansions))
		for _, existingCombo := range allCombinations {
			for _, expandedSeed := range expansions {
				comboCopy := make([][]byte, len(existingCombo)+1)
				copy(comboCopy, existingCombo)
				comboCopy[len(existingCombo)] = expandedSeed
				newCombinations = append(newCombinations, comboCopy)
			}
		}

		allCombinations = newCombinations
	}

	return allCombinations, nil
}

// generatePDAs generates program-derived addresses (PDAs) from public keys and seeds.
// it will result in a list of PDAs whose length is the product of the number of public keys
// and the number of seed combinations.
func generatePDAs(
	publicKeys []*solana.AccountMeta,
	seedCombos [][][]byte,
	lookup *PDALookups,
) ([]*solana.AccountMeta, error) {
	var results []*solana.AccountMeta
	for _, publicKeyMeta := range publicKeys {
		for _, combo := range seedCombos {
			if len(combo) > solana.MaxSeeds {
				return nil, fmt.Errorf("seed maximum exceeded: %d", len(combo))
			}
			address, _, err := solana.FindProgramAddress(combo, publicKeyMeta.PublicKey)
			if err != nil {
				return nil, fmt.Errorf("error finding program address: %w", err)
			}
			results = append(results, &solana.AccountMeta{
				PublicKey:  address,
				IsSigner:   lookup.IsSigner,
				IsWritable: lookup.IsWritable,
			})
		}
	}

	return results, nil
}

func lookupErrWithName(name string, err error) error {
	return fmt.Errorf("lookup: %s, err: %w", name, err)
}
