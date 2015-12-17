package client

import (
	"fmt"
	"strings"
)

// Copied from github.com/docker/docker/opts instead of using godeps
// which will include extra packages

//MapOpts holds a map of values and a validation function.
type MapOpts struct {
	values    map[string]string
	validator ValidatorFctType
}

// Set validates if needed the input value and add it to the
// internal map, by splitting on '='.
func (opts *MapOpts) Set(value string) error {
	if opts.validator != nil {
		v, err := opts.validator(value)
		if err != nil {
			return err
		}
		value = v
	}
	vals := strings.SplitN(value, "=", 2)
	if len(vals) == 1 {
		(opts.values)[vals[0]] = ""
	} else {
		(opts.values)[vals[0]] = vals[1]
	}
	return nil
}

// GetAll returns the values of MapOpts as a map.
func (opts *MapOpts) GetAll() map[string]string {
	return opts.values
}

func (opts *MapOpts) String() string {
	return fmt.Sprintf("%v", map[string]string((opts.values)))
}

// NewMapOpts creates a new MapOpts with the specified map of values and a validator.
func NewMapOpts(values map[string]string, validator ValidatorFctType) *MapOpts {
	if values == nil {
		values = make(map[string]string)
	}
	return &MapOpts{
		values:    values,
		validator: validator,
	}
}

// ValidatorFctType defines a validator function that returns a validated string and/or an error.
type ValidatorFctType func(val string) (string, error)
