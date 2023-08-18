package filewriter

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-codec/codec"
)

var jsonPrettyHandle *codec.JsonHandle

func init() {
	jsonPrettyHandle = new(codec.JsonHandle)
	jsonPrettyHandle.ErrorIfNoField = json.CodecHandle.ErrorIfNoField
	jsonPrettyHandle.ErrorIfNoArrayExpand = json.CodecHandle.ErrorIfNoArrayExpand
	jsonPrettyHandle.Canonical = json.CodecHandle.Canonical
	jsonPrettyHandle.RecursiveEmptyCheck = json.CodecHandle.RecursiveEmptyCheck
	jsonPrettyHandle.Indent = json.CodecHandle.Indent
	jsonPrettyHandle.HTMLCharsAsIs = json.CodecHandle.HTMLCharsAsIs
	jsonPrettyHandle.MapKeyAsString = true
	jsonPrettyHandle.Indent = 2
}

func ParseFilenamePattern(pattern string) (EncodingFormat, bool, error) {
	originalPattern := pattern
	gzip := false
	if strings.HasSuffix(pattern, ".gz") {
		gzip = true
		pattern = pattern[:len(pattern)-3]
	}

	var blockFormat EncodingFormat
	if strings.HasSuffix(pattern, ".msgp") {
		blockFormat = MessagepackFormat
	} else if strings.HasSuffix(pattern, ".json") {
		blockFormat = JSONFormat
	} else {
		return UnrecognizedFormat, false, fmt.Errorf("unrecognized export format: %s", originalPattern)
	}

	return blockFormat, gzip, nil
}

func GenesisFilename(format EncodingFormat, isGzip bool) (string, error) {
	var ext string
	
	switch format {
	case JSONFormat:
		ext = ".json"
	case MessagepackFormat:
		ext = ".msgp"
	default:
		return "", fmt.Errorf("GenesisFilename(): unhandled format %d", format)
	}

	if isGzip {
		ext += ".gz"
	}

	return fmt.Sprintf("genesis%s", ext), nil
}

// EncodeToFile enocods an object to a file using a given format and possible gzip compression.
func EncodeToFile(filename string, v interface{}, format EncodingFormat, isGzip bool) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("EncodeToFile(): failed to create %s: %w", filename, err)
	}
	defer file.Close()

	var writer io.Writer
	if isGzip {
		gz := gzip.NewWriter(file)
		gz.Name = filename
		defer gz.Close()
		writer = gz
	} else {
		writer = file
	}

	return Encode(format, writer, v)
}

func Encode(format EncodingFormat, writer io.Writer, v interface{}) error {
	var handle codec.Handle
	switch format {
	case JSONFormat:
		handle = jsonPrettyHandle
	case MessagepackFormat:
		handle = msgpack.LenientCodecHandle
	default:
		return fmt.Errorf("EncodeToFile(): unhandled format %d", format)
	}
	return codec.NewEncoder(writer, handle).Encode(v)
}

// DecodeFromFile decodes a file to an object using a given format and possible gzip compression.
func DecodeFromFile(filename string, v interface{}, format EncodingFormat, isGzip bool) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("DecodeFromFile(): failed to open %s: %w", filename, err)
	}
	defer file.Close()

	var reader io.Reader
	if isGzip {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("DecodeFromFile(): failed to make gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	} else {
		reader = file
	}

	var handle codec.Handle
	switch format {
	case JSONFormat:
		handle = json.LenientCodecHandle
	case MessagepackFormat:
		handle = msgpack.LenientCodecHandle
	default:
		return fmt.Errorf("DecodeFromFile(): unhandled format %d", format)
	}
	return codec.NewDecoder(reader, handle).Decode(v)
}
