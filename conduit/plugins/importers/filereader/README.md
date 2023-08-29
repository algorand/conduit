# File Import Plugin

Read files from a directory and import them as blocks. This plugin works with the file exporter plugin to create a simple file-based pipeline.

The genesis must be a plain JSON file named `genesis.json` regardless of the `FilenamePattern`.

## Configuration

```yml @sample.yaml
name: file_reader
config:
    # BlocksDir is the path to a directory where block data should be stored.
    # The directory is created if it doesn't exist. If no directory is provided
    # blocks are written to the Conduit data directory.
    #block-dir: "/path/to/directory"
        
    # FilenamePattern is the format used to find block files. It uses go string
    # formatting and should accept one number for the round.
    # The pattern should match the extension of the files to be read.
    filename-pattern: "%[1]d_block.msgp.gz"
```
