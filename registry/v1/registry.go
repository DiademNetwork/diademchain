package registry

import (
	"errors"
	"regexp"

	proto "github.com/gogo/protobuf/proto"
	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/util"
	"github.com/diademnetwork/diademchain"
	common "github.com/diademnetwork/diademchain/registry"
)

const (
	minNameLen = 1
	maxNameLen = 255
)

var (
	validNameRE = regexp.MustCompile("^[a-zA-Z0-9\\.\\-]+$")
)

func recordKey(name string) []byte {
	return util.PrefixKey([]byte("registry"), []byte(name))
}

// StateRegistry stores contract meta data for named contracts only, and allows lookup by contract name.
type StateRegistry struct {
	State diademchain.State
}

var _ common.Registry = &StateRegistry{}

func (r *StateRegistry) Register(name string, addr, owner diadem.Address) error {
	// In previous builds this function was only called when the name wasn't empty, so to maintain
	// backward compatibility do nothing if the name is empty.
	if name == "" {
		return nil
	}

	err := validateName(name)
	if err != nil {
		return err
	}

	_, err = r.Resolve(name)
	if err == nil {
		return common.ErrAlreadyRegistered
	}
	if err != common.ErrNotFound {
		return err
	}

	data, err := proto.Marshal(&common.Record{
		Name:    name,
		Owner:   owner.MarshalPB(),
		Address: addr.MarshalPB(),
	})
	if err != nil {
		return err
	}
	r.State.Set(recordKey(name), data)
	return nil
}

func (r *StateRegistry) Resolve(name string) (diadem.Address, error) {
	data := r.State.Get(recordKey(name))
	if len(data) == 0 {
		return diadem.Address{}, common.ErrNotFound
	}
	var record common.Record
	err := proto.Unmarshal(data, &record)
	if err != nil {
		return diadem.Address{}, err
	}
	return diadem.UnmarshalAddressPB(record.Address), nil
}

func (r *StateRegistry) GetRecord(contractAddr diadem.Address) (*common.Record, error) {
	return nil, common.ErrNotImplemented
}

func validateName(name string) error {
	if len(name) < minNameLen {
		return errors.New("name length too short")
	}

	if len(name) > maxNameLen {
		return errors.New("name length too long")
	}

	if !validNameRE.MatchString(name) {
		return errors.New("invalid name format")
	}

	return nil
}
