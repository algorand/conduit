package importers

import (
	"fmt"
)

// ImporterConstructor must be implemented by each Importer.
// It provides a basic no-arg constructor for instances of an ImporterImpl.
type ImporterConstructor interface {
	// New should return an instantiation of a Importer.
	// Configuration values should be passed and can be processed during `Init()`.
	New() Importer
}

// ImporterConstructorFunc is Constructor implementation for importers
type ImporterConstructorFunc func() Importer

// New initializes an importer constructor
func (f ImporterConstructorFunc) New() Importer {
	return f()
}

// Importers are the constructors to build importer plugins.
var Importers = make(map[string]ImporterConstructor)

// Register is used to register Constructor implementations. This mechanism allows
// for loose coupling between the configuration and the implementation. It is extremely similar to the way sql.DB
// drivers are configured and used.
func Register(name string, constructor ImporterConstructor) {
	if _, ok := Importers[name]; ok {
		panic(fmt.Errorf("importer %s already registered", name))
	}
	Importers[name] = constructor
}

// ImporterConstructorByName returns a Importer constructor for the name provided
func ImporterConstructorByName(name string) (ImporterConstructor, error) {
	constructor, ok := Importers[name]
	if !ok {
		return nil, fmt.Errorf("no Importer Constructor for %s", name)
	}

	return constructor, nil
}
