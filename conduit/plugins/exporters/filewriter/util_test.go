package filewriter

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)


func TestParseFilenameFormat(t *testing.T) {
	testCases := []struct {
		name     string
		format   string
		gzip	 bool
		blockFormat EncodingFormat
		err 	string
	}{
		{
			name: "messagepack vanilla",
			format: "%d_block.msgp",		
			gzip: false,
			blockFormat: MessagepackFormat,
			err: "",
		},
		{
			name: "messagepack gzip",
			format: "%d_block.msgp.gz",
			gzip: true,
			blockFormat: MessagepackFormat,
			err: "",
		},
		{
			name: "json vanilla",
			format: "%d_block.json",
			gzip: false,
			blockFormat: JSONFormat,
			err: "",
		},
		{
			name: "json gzip",	
			format: "%d_block.json.gz",
			gzip: true,
			blockFormat: JSONFormat,
			err: "",
		},
		{
			name: "messagepack vanilla 2",
			format: "%[1]d_block round%[1]d.msgp",		
			gzip: false,
			blockFormat: MessagepackFormat,
			err: "",
		},
		{
			name: "messagepack gzip 2",
			format: "%[1]d_block round%[1]d.msgp.gz",
			gzip: true,
			blockFormat: MessagepackFormat,
			err: "",
		},
		{
			name: "json vanilla 2",
			format: "%[1]d_block round%[1]d.json",
			gzip: false,
			blockFormat: JSONFormat,
			err: "",
		},
		{
			name: "json gzip 2",	
			format: "%[1]d_block round%[1]d.json.gz",
			gzip: true,
			blockFormat: JSONFormat,
			err: "",
		},
		{
			name: "invalid - gzip",
			format: "%d_block.msgp.gzip",
			gzip: false,
			blockFormat: UnrecognizedFormat,
			err: "unrecognized export format",
		},
		{
			name: "invalid - no extension",
			format: "%d_block",
			gzip: false,
			blockFormat: UnrecognizedFormat,
			err: "unrecognized export format",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			blockFormat, gzip, err := ParseFilenamePattern(tc.format)
			if tc.err == "" {	
				require.NoError(t, err)
				require.Equal(t, tc.gzip, gzip)
				require.Equal(t, tc.blockFormat, blockFormat)
			} else {
				require.ErrorContains(t, err, tc.err)
			}
		})
	}
}

func TestGenesisFilename(t *testing.T) {
	testCases := []struct {
		blockFormat EncodingFormat
		gzip   bool
		result string
		err 	string
	}{
		{
			blockFormat: MessagepackFormat,
			gzip: false,
			result: "genesis.msgp",
			err: "",
		},
		{
			blockFormat: MessagepackFormat,
			gzip: true,
			result: "genesis.msgp.gz",
			err: "",
		},
		{
			blockFormat: JSONFormat,
			gzip: false,
			result: "genesis.json",
			err: "",
		},
		{
			blockFormat: JSONFormat,
			gzip: true,
			result: "genesis.json.gz",
			err: "",
		},
		{
			result: "error case",
			blockFormat: EncodingFormat(42),
			err: "GenesisFilename(): unhandled format 42",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.result, func(t *testing.T) {
			t.Parallel()

			filename, err := GenesisFilename(tc.blockFormat, tc.gzip)
			if tc.err == "" {	
				require.NoError(t, err)
				require.Equal(t, tc.result, filename)
			} else {
				require.ErrorContains(t, err, tc.err)
			}
		})
	}
}

func TestEncodeToAndFromFile(t *testing.T) {
	tempdir := t.TempDir()

	type test struct {
		One    string         `json:"one"`
		Two    int            `json:"two"`
		Strict map[int]string `json:"strict"`
	}
	data := test{
		One: "one",
		Two: 2,
		Strict: map[int]string{
			0: "int-key",
		},
	}

	{
		jsonFile := path.Join(tempdir, "json.json")
		err := EncodeToFile(jsonFile, data, JSONFormat, false)
		require.NoError(t, err)
		require.FileExists(t, jsonFile)
		var testDecode test
		err = DecodeFromFile(jsonFile, &testDecode, JSONFormat, false)
		require.NoError(t, err)
		require.Equal(t, data, testDecode)

		// Check the pretty printing
		bytes, err := os.ReadFile(jsonFile)
		require.NoError(t, err)
		require.Contains(t, string(bytes), "  \"one\": \"one\",\n")
		require.Contains(t, string(bytes), `"0": "int-key"`)
	}

	{
		small := path.Join(tempdir, "small.json")
		err := EncodeToFile(small, data, JSONFormat, false)
		require.NoError(t, err)
		require.FileExists(t, small)
		var testDecode test
		err = DecodeFromFile(small, &testDecode, JSONFormat, false)
		require.NoError(t, err)
		require.Equal(t, data, testDecode)
	}

	// gzip test
	{
		small := path.Join(tempdir, "small.json.gz")
		err := EncodeToFile(small, data, JSONFormat, true)
		require.NoError(t, err)
		require.FileExists(t, small)
		var testDecode test
		err = DecodeFromFile(small, &testDecode, JSONFormat, true)
		require.NoError(t, err)
		require.Equal(t, data, testDecode)
	}
}
