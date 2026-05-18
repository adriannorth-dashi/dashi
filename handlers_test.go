package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/adrian_north/dashi/testutils"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// nullDB returns a *DB backed by an unreachable Postgres port.
// sql.Open is lazy — no actual connection until first query.
// When a handler calls LogSponsorship, it gets ECONNREFUSED immediately
// and the error is silently discarded (handlers.go line 67: _ = c.Error(logErr)).
func nullDB(t *testing.T) *DB {
	t.Helper()
	conn, err := sql.Open("pgx", "postgres://null:null@127.0.0.1:19999/null")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	return &DB{conn: conn}
}

// newTestHandlers wires up a Handlers instance pointing at mock servers.
// Pass nil for gasPool or suiRPC to use an empty URL (endpoint unreachable).
func newTestHandlers(t *testing.T, gasPool, suiRPC *httptest.Server) *Handlers {
	t.Helper()
	gasURL, suiURL := "", "http://127.0.0.1:19998"
	if gasPool != nil {
		gasURL = gasPool.URL
	}
	if suiRPC != nil {
		suiURL = suiRPC.URL
	}
	return &Handlers{
		db:      nullDB(t),
		dashi: NewDashiClient(gasURL, "test-token", ""),
		sui:     NewSuiClient(suiURL, ""),
		cfg: Config{
			Network: "testnet",
			APIKey:  testutils.TestAPIKey,
		},
	}
}

// newTestRouter delegates to the production newRouter so tests cover the same code path.
func newTestRouter(h *Handlers) *gin.Engine {
	return newRouter(h)
}

func TestNewRouter_RegistersAllRoutes(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newRouter(h)

	routes := r.Routes()
	want := map[string]string{
		"GET /health":             "",
		"POST /v1/sponsor":        "",
		"POST /v1/execute":        "",
		"GET /v1/sponsor/:digest": "",
		"GET /v1/balance":         "",
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		delete(want, key)
	}
	if len(want) > 0 {
		t.Errorf("missing routes: %v", want)
	}
}

// do sends an HTTP request through the router and returns the recorder.
func do(t *testing.T, r *gin.Engine, method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func authHeader() map[string]string {
	return map[string]string{"X-API-Key": testutils.TestAPIKey}
}

func parseBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("could not parse response body as JSON: %v\nbody: %s", err, w.Body.String())
	}
	return m
}

// ── GET /health ───────────────────────────────────────────────────────────────

func TestHealth_Returns200WithStatusOk(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	w := do(t, r, "GET", "/health", nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := parseBody(t, w)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
	if body["network"] != "testnet" {
		t.Errorf("expected network=testnet, got %v", body["network"])
	}
}

func TestHealth_NoAPIKeyRequired(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	w := do(t, r, "GET", "/health", nil, nil) // no X-API-Key
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 without API key, got %d", w.Code)
	}
}

// ── POST /v1/sponsor ──────────────────────────────────────────────────────────

func TestSponsorTransaction_ValidRequest(t *testing.T) {
	gasPool := testutils.MockGasPoolServer(t)
	suiRPC := testutils.MockSuiRPC(t)
	h := newTestHandlers(t, gasPool, suiRPC)
	r := newTestRouter(h)

	body := map[string]string{
		"transactionKindBytes": "AQIDBA==",
		"sender":               testutils.ValidSuiAddress(),
	}
	w := do(t, r, "POST", "/v1/sponsor", body, authHeader())

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := parseBody(t, w)
	for _, field := range []string{"sponsoredTransaction", "sponsorshipId", "feeInfo"} {
		if _, ok := resp[field]; !ok {
			t.Errorf("expected field %q in response, body: %s", field, w.Body.String())
		}
	}
	// sponsoredTransaction is now base64 TransactionData bytes, not a digest.
	if txBytes, _ := resp["sponsoredTransaction"].(string); txBytes == "" {
		t.Error("sponsoredTransaction must be a non-empty base64 string")
	}
	// sponsorshipId is the numeric reservation ID.
	if _, ok := resp["sponsorshipId"].(float64); !ok {
		t.Errorf("sponsorshipId must be a number, got %T", resp["sponsorshipId"])
	}
	feeInfo, ok := resp["feeInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("feeInfo is not an object")
	}
	for _, fee := range []string{"networkFee", "serviceFee", "totalFee"} {
		if _, ok := feeInfo[fee]; !ok {
			t.Errorf("expected feeInfo.%s in response", fee)
		}
	}
}

func TestSponsorTransaction_GasPoolFailure_Returns502(t *testing.T) {
	gasPool := testutils.MockGasPoolServerError(t)
	h := newTestHandlers(t, gasPool, nil)
	r := newTestRouter(h)

	body := map[string]string{
		"transactionKindBytes": "AQIDBA==",
		"sender":               testutils.ValidSuiAddress(),
	}
	w := do(t, r, "POST", "/v1/sponsor", body, authHeader())

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 on gas-pool failure, got %d", w.Code)
	}
}

func TestSponsorTransaction_MissingTransactionKindBytes(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	body := map[string]string{"sender": testutils.ValidSuiAddress()}
	w := do(t, r, "POST", "/v1/sponsor", body, authHeader())

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSponsorTransaction_MissingSender(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	body := map[string]string{"transactionKindBytes": "AQIDBA=="}
	w := do(t, r, "POST", "/v1/sponsor", body, authHeader())

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSponsorTransaction_InvalidSuiAddresses(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	for _, tc := range testutils.InvalidSuiAddresses() {
		t.Run(tc.Name, func(t *testing.T) {
			body := map[string]string{
				"transactionKindBytes": "AQIDBA==",
				"sender":               tc.Address,
			}
			w := do(t, r, "POST", "/v1/sponsor", body, authHeader())
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for %q, got %d", tc.Address, w.Code)
			}
		})
	}
}

func TestSponsorTransaction_NoAPIKey_Returns401(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	body := map[string]string{
		"transactionKindBytes": "AQIDBA==",
		"sender":               testutils.ValidSuiAddress(),
	}
	w := do(t, r, "POST", "/v1/sponsor", body, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSponsorTransaction_WrongAPIKey_Returns401(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	body := map[string]string{
		"transactionKindBytes": "AQIDBA==",
		"sender":               testutils.ValidSuiAddress(),
	}
	w := do(t, r, "POST", "/v1/sponsor", body, map[string]string{"X-API-Key": "wrong-key"})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// ── GET /v1/sponsor/:digest ───────────────────────────────────────────────────

func TestGetSponsorStatus_ValidDigest(t *testing.T) {
	suiRPC := testutils.MockSuiRPC(t)
	h := newTestHandlers(t, nil, suiRPC)
	r := newTestRouter(h)

	w := do(t, r, "GET", "/v1/sponsor/"+testutils.TestTxDigest, nil, authHeader())

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := parseBody(t, w)
	if _, ok := resp["status"]; !ok {
		t.Error("expected status field in response")
	}
	if _, ok := resp["digest"]; !ok {
		t.Error("expected digest field in response")
	}
}

func TestGetSponsorStatus_NoAPIKey_Returns401(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	w := do(t, r, "GET", "/v1/sponsor/somedigest", nil, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestGetSponsorStatus_EmptyDigest_Returns404(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	// /v1/sponsor/ with no segment matches no route → 404
	w := do(t, r, "GET", "/v1/sponsor/", nil, authHeader())
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty digest path, got %d", w.Code)
	}
}

func TestGetSponsorStatus_EmptyDigestDirectCall_Returns400(t *testing.T) {
	// Call the handler directly (bypassing gin routing) to exercise the
	// `if digest == ""` guard which gin routing never reaches via HTTP.
	h := newTestHandlers(t, nil, nil)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/v1/sponsor/", nil)
	// No Params set → c.Param("digest") returns ""

	h.GetSponsorStatus(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty digest (direct call), got %d", w.Code)
	}
}

// ── GET /v1/balance ───────────────────────────────────────────────────────────

func TestGetBalance_Returns200WithBalanceField(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	w := do(t, r, "GET", "/v1/balance", nil, authHeader())

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	resp := parseBody(t, w)
	if _, ok := resp["balance"]; !ok {
		t.Error("expected balance field in response")
	}
	if resp["currency"] != "SUI" {
		t.Errorf("expected currency=SUI, got %v", resp["currency"])
	}
}

func TestGetBalance_NoAPIKey_Returns401(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	w := do(t, r, "GET", "/v1/balance", nil, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// ── POST /v1/execute ──────────────────────────────────────────────────────────

func TestExecuteSponsored_ValidRequest(t *testing.T) {
	gasPool := testutils.MockGasPoolServer(t)
	h := newTestHandlers(t, gasPool, nil)
	r := newTestRouter(h)

	body := map[string]interface{}{
		"sponsorshipId": 12345,
		"txBytes":       "AQIDBA==",
		"userSig":       "dGVzdA==",
	}
	w := do(t, r, "POST", "/v1/execute", body, authHeader())

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := parseBody(t, w)
	if _, ok := resp["digest"]; !ok {
		t.Error("expected digest field in response")
	}
	if _, ok := resp["status"]; !ok {
		t.Error("expected status field in response")
	}
}

func TestExecuteSponsored_GasPoolFailure_Returns502(t *testing.T) {
	gasPool := testutils.MockGasPoolServerError(t)
	h := newTestHandlers(t, gasPool, nil)
	r := newTestRouter(h)

	body := map[string]interface{}{
		"sponsorshipId": 12345,
		"txBytes":       "AQIDBA==",
		"userSig":       "dGVzdA==",
	}
	w := do(t, r, "POST", "/v1/execute", body, authHeader())

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 on gas-pool failure, got %d", w.Code)
	}
}

func TestExecuteSponsored_MissingFields_Returns400(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	// Missing userSig
	body := map[string]interface{}{
		"sponsorshipId": 12345,
		"txBytes":       "AQIDBA==",
	}
	w := do(t, r, "POST", "/v1/execute", body, authHeader())

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing fields, got %d", w.Code)
	}
}

func TestExecuteSponsored_NoAPIKey_Returns401(t *testing.T) {
	h := newTestHandlers(t, nil, nil)
	r := newTestRouter(h)

	body := map[string]interface{}{
		"sponsorshipId": 12345,
		"txBytes":       "AQIDBA==",
		"userSig":       "dGVzdA==",
	}
	w := do(t, r, "POST", "/v1/execute", body, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
