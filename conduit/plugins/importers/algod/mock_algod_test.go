package algodimporter

import (
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

// algodCustomHandler is used by a silly muxer we created which brute forces the path and returns true if it handles the request.
type algodCustomHandler = func(r *http.Request, w http.ResponseWriter) bool

// AlgodHandler is used to handle http requests to a mock algod server
type AlgodHandler struct {
	responders []algodCustomHandler
}

// NewAlgodServer creates an httptest server with an algodHandler using the provided responders
func NewAlgodServer(responders ...algodCustomHandler) *httptest.Server {
	return httptest.NewServer(&AlgodHandler{responders})
}

// NewAlgodHandler creates an AlgodHandler using the provided responders
func NewAlgodHandler(responders ...algodCustomHandler) *AlgodHandler {
	return &AlgodHandler{responders}
}

// ServeHTTP implements the http.Handler interface for AlgodHandler
func (handler *AlgodHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for _, responder := range handler.responders {
		if responder(req, w) {
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}

// BlockResponder handles /v2/blocks requests and returns an empty Block object
func BlockResponder(r *http.Request, w http.ResponseWriter) bool {
	if strings.Contains(r.URL.Path, "v2/blocks/") {
		rnd, _ := strconv.Atoi(path.Base(r.URL.Path))
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
func MakeGenesisResponder(genesis types.Genesis) algodCustomHandler {
	return MakeJsonResponder("/genesis", genesis)
}

// GenesisResponder handles /v2/genesis requests and returns an empty Genesis object
var GenesisResponder = MakeGenesisResponder(types.Genesis{
	Comment: "",
	Network: "FAKE",
	DevMode: true,
})

func MakeBlockAfterResponder(status models.NodeStatus) algodCustomHandler {
	return MakeJsonResponder("/wait-for-block-after", status)
}

// MakeJsonResponderSeries creates a series of responses with the provided http statuses and objects
func MakeJsonResponderSeries(url string, responseSeries []int, responseObjects []interface{}) algodCustomHandler {
	var i, j = 0, 0
	return func(r *http.Request, w http.ResponseWriter) bool {
		if strings.Contains(r.URL.Path, url) {
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

func MakeGetSyncRoundResponder(httpStatus int, round uint64) algodCustomHandler {
	return MakeJsonStatusResponder("get", "/v2/ledger/sync", httpStatus, models.GetSyncRoundResponse{
		Round: round,
	})
}

func MakePostSyncRoundResponder(httpStatus int) algodCustomHandler {
	return MakeMsgpStatusResponder("post", "/v2/ledger/sync", httpStatus, "")
}

func MakeNodeStatusResponder(status models.NodeStatus) algodCustomHandler {
	return MakeJsonResponder("/v2/status", status)
}

var BlockAfterResponder = MakeBlockAfterResponder(models.NodeStatus{})

func MakeLedgerStateDeltaResponder(delta types.LedgerStateDelta) algodCustomHandler {
	return MakeMsgpResponder("/v2/deltas/", delta)
}

var LedgerStateDeltaResponder = MakeLedgerStateDeltaResponder(types.LedgerStateDelta{})

func MakeJsonResponder(url string, object interface{}) algodCustomHandler {
	return MakeJsonStatusResponder("get", url, http.StatusOK, object)
}

func MakeJsonStatusResponder(method, url string, status int, object interface{}) algodCustomHandler {
	return func(r *http.Request, w http.ResponseWriter) bool {
		if strings.EqualFold(method, r.Method) && strings.Contains(r.URL.Path, url) {
			w.WriteHeader(status)
			_, _ = w.Write(json.Encode(object))
			return true
		}
		return false
	}
}

func MakeMsgpResponder(url string, object interface{}) algodCustomHandler {
	return MakeMsgpStatusResponder("get", url, http.StatusOK, object)
}

func MakeMsgpStatusResponder(method, url string, status int, object interface{}) algodCustomHandler {
	return func(r *http.Request, w http.ResponseWriter) bool {
		if strings.EqualFold(method, r.Method) && strings.Contains(r.URL.Path, url) {
			w.WriteHeader(status)
			_, _ = w.Write(msgpack.Encode(object))
			return true
		}
		return false
	}
}
