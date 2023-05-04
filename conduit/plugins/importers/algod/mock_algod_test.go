package algodimporter

import (
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

// AlgodHandler is used to handle http requests to a mock algod server
type AlgodHandler struct {
	responders []func(path string, w http.ResponseWriter) bool
}

// NewAlgodServer creates an httptest server with an algodHandler using the provided responders
func NewAlgodServer(responders ...func(path string, w http.ResponseWriter) bool) *httptest.Server {
	return httptest.NewServer(&AlgodHandler{responders})
}

// NewAlgodHandler creates an AlgodHandler using the provided responders
func NewAlgodHandler(responders ...func(path string, w http.ResponseWriter) bool) *AlgodHandler {
	return &AlgodHandler{responders}
}

// ServeHTTP implements the http.Handler interface for AlgodHandler
func (handler *AlgodHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for _, responder := range handler.responders {
		if responder(req.URL.Path, w) {
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}

// MockAClient creates an algod client using an AlgodHandler based server
func MockAClient(handler *AlgodHandler) (*algod.Client, error) {
	mockServer := httptest.NewServer(handler)
	return algod.MakeClient(mockServer.URL, "")
}

// BlockResponder handles /v2/blocks requests and returns an empty Block object
func BlockResponder(reqPath string, w http.ResponseWriter) bool {
	if strings.Contains(reqPath, "v2/blocks/") {
		rnd, _ := strconv.Atoi(path.Base(reqPath))
		type EncodedBlock struct {
			_struct struct{}    `codec:""`
			Block   types.Block `codec:"block"`
		}
		blk := EncodedBlock{Block: types.Block{BlockHeader: types.BlockHeader{Round: types.Round(rnd)}}}
		blockbytes := msgpack.Encode(&blk)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(blockbytes)
		return true
	}
	return false
}

// MakeGenesisResponder returns a responder that will provide a specific genesis response.
func MakeGenesisResponder(genesis types.Genesis) func(reqPath string, w http.ResponseWriter) bool {
	return MakeJsonResponder("/genesis", genesis)
}

// GenesisResponder handles /v2/genesis requests and returns an empty Genesis object
var GenesisResponder = MakeGenesisResponder(types.Genesis{
	Comment: "",
	Network: "FAKE",
	DevMode: true,
})

func MakeBlockAfterResponder(status models.NodeStatus) func(string, http.ResponseWriter) bool {
	return MakeJsonResponder("/wait-for-block-after", status)
}

// MakeJsonResponderSeries creates a series of responses with the provided http statuses and objects
func MakeJsonResponderSeries(url string, responseSeries []int, responseObjects []interface{}) func(string, http.ResponseWriter) bool {
	var i, j = 0, 0
	return func(reqPath string, w http.ResponseWriter) bool {
		if strings.Contains(reqPath, url) {
			w.WriteHeader(responseSeries[i])
			_, _ = w.Write(json.Encode(responseObjects[j]))
			if i < len(responseSeries)-1 {
				i = i + 1
			}
			if j < len(responseObjects)-1 {
				j = j + 1
			}
			return true
		}
		return false
	}
}

func MakeSyncRoundResponder(httpStatus int) func(string, http.ResponseWriter) bool {
	return MakeStatusResponder("/v2/ledger/sync", httpStatus, "")
}

func MakeNodeStatusResponder(status models.NodeStatus) func(string, http.ResponseWriter) bool {
	return MakeJsonResponder("/v2/status", status)
}

var BlockAfterResponder = MakeBlockAfterResponder(models.NodeStatus{})

func MakeLedgerStateDeltaResponder(delta types.LedgerStateDelta) func(string, http.ResponseWriter) bool {
	return MakeMsgpResponder("/v2/deltas/", delta)
}

var LedgerStateDeltaResponder = MakeLedgerStateDeltaResponder(types.LedgerStateDelta{})

func MakeJsonResponder(url string, object interface{}) func(string, http.ResponseWriter) bool {
	return func(reqPath string, w http.ResponseWriter) bool {
		if strings.Contains(reqPath, url) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(json.Encode(object))
			return true
		}
		return false
	}
}

func MakeMsgpResponder(url string, object interface{}) func(string, http.ResponseWriter) bool {
	return func(reqPath string, w http.ResponseWriter) bool {
		if strings.Contains(reqPath, url) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(msgpack.Encode(object))
			return true
		}
		return false
	}
}

func MakeStatusResponder(url string, status int, object interface{}) func(string, http.ResponseWriter) bool {
	return func(reqPath string, w http.ResponseWriter) bool {
		if strings.Contains(reqPath, url) {
			w.WriteHeader(status)
			_, _ = w.Write(msgpack.Encode(object))
			return true
		}
		return false
	}
}
