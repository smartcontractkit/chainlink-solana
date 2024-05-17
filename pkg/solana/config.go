package solana

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/exp/slices"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	relaytypes "github.com/smartcontractkit/chainlink-common/pkg/types"

	solcfg "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

type TOMLConfigs []*solcfg.TOMLConfig

func (cs TOMLConfigs) ValidateConfig() (err error) {
	return cs.validateKeys()
}

func (cs TOMLConfigs) validateKeys() (err error) {
	// Unique chain IDs
	chainIDs := config.UniqueStrings{}
	for i, c := range cs {
		if chainIDs.IsDupe(c.ChainID) {
			err = errors.Join(err, config.NewErrDuplicate(fmt.Sprintf("%d.ChainID", i), *c.ChainID))
		}
	}

	// Unique node names
	names := config.UniqueStrings{}
	for i, c := range cs {
		for j, n := range c.Nodes {
			if names.IsDupe(n.Name) {
				err = errors.Join(err, config.NewErrDuplicate(fmt.Sprintf("%d.Nodes.%d.Name", i, j), *n.Name))
			}
		}
	}

	// Unique URLs
	urls := config.UniqueStrings{}
	for i, c := range cs {
		for j, n := range c.Nodes {
			u := (*url.URL)(n.URL)
			if urls.IsDupeFmt(u) {
				err = errors.Join(err, config.NewErrDuplicate(fmt.Sprintf("%d.Nodes.%d.URL", i, j), u.String()))
			}
		}
	}
	return
}

func (cs *TOMLConfigs) SetFrom(fs *TOMLConfigs) (err error) {
	if err1 := fs.validateKeys(); err1 != nil {
		return err1
	}
	for _, f := range *fs {
		if f.ChainID == nil {
			*cs = append(*cs, f)
		} else if i := slices.IndexFunc(*cs, func(c *solcfg.TOMLConfig) bool {
			return c.ChainID != nil && *c.ChainID == *f.ChainID
		}); i == -1 {
			*cs = append(*cs, f)
		} else {
			(*cs)[i].SetFrom(f)
		}
	}
	return
}

func nodeStatus(n *solcfg.Node, id string) (relaytypes.NodeStatus, error) {
	var s relaytypes.NodeStatus
	s.ChainID = id
	s.Name = *n.Name
	b, err := toml.Marshal(n)
	if err != nil {
		return relaytypes.NodeStatus{}, err
	}
	s.Config = string(b)
	return s, nil
}
