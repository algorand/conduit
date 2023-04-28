package algodimporter

//go:generate go run ../../../../cmd/conduit-docs/main.go ../../../../conduit-docs/
//go:generate go run ../../../../cmd/readme_config_includer/generator.go

//Name: conduit_importers_algod

// Config specific to the algod importer
type Config struct {
	// <code>mode</code> is the mode of operation of the algod importer.  It must be either <code>archival</code> or <code>follower</code>.
	Mode string `yaml:"mode"`
	// <code>netaddr</code> is the Algod network address. It must be either an <code>http</code> or <code>https</code> URL.
	NetAddr string `yaml:"netaddr"`
	// <code>token</code> is the Algod API endpoint token.
	Token string `yaml:"token"`
	// <code>catchup-config</code> is an optional set of parameters used to catchup an algod node in follower mode with the pipeline round on startup.
	CatchupConfig CatchupParams `yaml:"catchup-config"`
}

// CatchupParams provides information required to sync a follower node to the pipeline round
type CatchupParams struct {
	// <code>auto</code> controls whether to automatically download an appropriate catchpoint label.
	Auto bool `yaml:"auto"`
	// <code>catchpoint</code> is the catchpoint used to run fast catchup on startup when your node is behind the current pipeline round.
	Catchpoint string `yaml:"catchpoint"`
	// <code>admin-token</code> is the algod admin API token.
	AdminToken string `yaml:"admin-token"`
}
