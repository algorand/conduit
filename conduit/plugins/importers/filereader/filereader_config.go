package fileimporter

//go:generate go run ../../../../cmd/conduit-docs/main.go ../../../../conduit-docs/
//go:generate go run ../../../../cmd/readme_config_includer/generator.go

//Name: conduit_importers_filereader

// Config specific to the file importer
type Config struct {
	// <code>block-dir</code> is the path to a directory where block data is stored.
	BlocksDir string `yaml:"block-dir"`

	/* <code>filename-pattern</code> is the format used to find block files. It uses go string formatting and should accept one number for the round.
	The default pattern is

	"%[1]d_block.json"
	*/
	FilenamePattern string `yaml:"filename-pattern"`

	// TODO: Option to delete files after processing them
}
