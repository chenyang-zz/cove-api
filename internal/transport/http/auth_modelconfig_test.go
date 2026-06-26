package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterLoginAndCreateModelConfig(t *testing.T) {
	router := newTestRouter(t)

	register := httptest.NewRecorder()
	registerReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"alice","email":"a@example.com","password":"secret123"}`))
	registerReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(register, registerReq)
	if register.Code != http.StatusOK {
		t.Fatalf("register status = %d body=%s", register.Code, register.Body.String())
	}

	login := httptest.NewRecorder()
	legacyLogin := httptest.NewRecorder()
	legacyLoginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"a@example.com","password":"secret123"}`))
	legacyLoginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(legacyLogin, legacyLoginReq)
	if legacyLogin.Code != http.StatusBadRequest {
		t.Fatalf("legacy login status = %d body=%s", legacyLogin.Code, legacyLogin.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"login":"alice","password":"secret123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, loginReq)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	var loginBody struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("unmarshal login: %v", err)
	}
	if loginBody.Data.AccessToken == "" {
		t.Fatal("access token is empty")
	}
	if loginBody.Data.RefreshToken == "" {
		t.Fatal("refresh token is empty")
	}

	refresh := httptest.NewRecorder()
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(`{"refresh_token":"`+loginBody.Data.RefreshToken+`"}`))
	refreshReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(refresh, refreshReq)
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body=%s", refresh.Code, refresh.Body.String())
	}
	var refreshBody struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(refresh.Body.Bytes(), &refreshBody); err != nil {
		t.Fatalf("unmarshal refresh: %v", err)
	}
	if refreshBody.Data.AccessToken == "" || refreshBody.Data.RefreshToken == "" {
		t.Fatalf("refresh tokens must be returned: %+v", refreshBody.Data)
	}
	if refreshBody.Data.RefreshToken == loginBody.Data.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/model-configs/create", strings.NewReader(`{"type":"chat","provider":"deepseek","name":"DeepSeek Chat","model_name":"deepseek-chat","base_url":"https://api.deepseek.com","api_key":"sk-secret","capability":["function_call"],"is_default":true}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+refreshBody.Data.AccessToken)
	router.ServeHTTP(create, createReq)
	if create.Code != http.StatusOK {
		t.Fatalf("create model config status = %d body=%s", create.Code, create.Body.String())
	}
	if strings.Contains(create.Body.String(), "sk-secret") {
		t.Fatalf("model config response leaked plaintext key: %s", create.Body.String())
	}
	if !strings.Contains(create.Body.String(), `"api_key_masked":"*****cret"`) {
		t.Fatalf("model config response missing masked key: %s", create.Body.String())
	}
	if !strings.Contains(create.Body.String(), `"model_name":"deepseek-chat"`) || !strings.Contains(create.Body.String(), `"capability":["`) {
		t.Fatalf("model config response missing new schema fields: %s", create.Body.String())
	}
}
