package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"time"
)

const (
	baseURL  = "https://navi.cap.by"
	loginURL = "https://navi.cap.by/api/v3/auth/login"

	login    = "test_cola"
	password = "111111"

	// agentId = "947a60ea-3ccf-46c7-866a-42f6ab0ac793"
	agentId = "a2dcf51b-26d1-4c95-b9f3-292a2c099bd6"
)

type CreateRequest struct {
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Radius float64 `json:"radius"`
}

type AuthResponse struct {
	AuthId string `json:"AuthId"`
	User   string `json:"User"`
}

type API struct {
	client *http.Client
	token  string
}

func NewAPI() *API {
	jar, _ := cookiejar.New(nil)

	return &API{
		client: &http.Client{
			Jar:     jar,
			Timeout: 20 * time.Second,
		},
	}
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Auth, X-Agent")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
}

func (a *API) login() error {
	body := map[string]string{
		"login":    login,
		"password": password,
	}

	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var auth AuthResponse
	if err := json.Unmarshal(data, &auth); err != nil {
		return err
	}

	if auth.AuthId == "" {
		return fmt.Errorf("empty auth id")
	}

	a.token = auth.AuthId

	u, _ := url.Parse(baseURL)
	a.client.Jar.SetCookies(u, []*http.Cookie{
		{
			Name:  "ext_AuthId",
			Value: auth.AuthId,
			Path:  "/",
		},
	})

	log.Println("LOGIN OK")
	log.Println("LOGIN RESPONSE TOKEN:", auth.AuthId)
	log.Println("USING AGENT:", agentId)
	log.Println("USER TOKEN:", a.token)
	return nil
}

func (a *API) createGeozone(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)

	if r.Method == "OPTIONS" {
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	if a.token == "" {
		if err := a.login(); err != nil {
			http.Error(w, "auth failed: "+err.Error(), 500)
			return
		}
	}

	shapeObj := map[string]interface{}{
		"type":        "Point",
		"coordinates": []float64{req.Lon, req.Lat},
	}

	shapeBytes, err := json.Marshal(shapeObj)
	if err != nil {
		http.Error(w, "shape error", 500)
		return
	}

	payload := map[string]interface{}{
		"agentId":      agentId,
		"name":         req.Name,
		"area":         0,
		"beginCalc":    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"description":  "",
		"display":      "",
		"endCalc":      nil,
		"id":           nil,
		"perimetr":     0,
		"radius":       req.Radius,
		"shape":        string(shapeBytes),
		"shape_format": "geojson",
		"type":         0,
		"unitId":       nil,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "marshal error", 500)
		return
	}

	httpReq, err := http.NewRequest("POST", baseURL+"/api/gis/", bytes.NewBuffer(b))
	if err != nil {
		http.Error(w, "request build error", 500)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json;charset=utf-8")
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")
	httpReq.Header.Set("X-Auth", a.token)
	httpReq.Header.Set("X-Agent", agentId)
	httpReq.Header.Set("Origin", baseURL)
	httpReq.Header.Set("Referer", baseURL+"/client.html?agentId="+agentId)

	httpReq.AddCookie(&http.Cookie{
		Name:  "ext_AuthId",
		Value: a.token,
		Path:  "/",
	})

	resp, err := a.client.Do(httpReq)
	if err != nil {
		http.Error(w, "request failed: "+err.Error(), 500)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "read error", 500)
		return
	}

	log.Println("STATUS:", resp.Status)

	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func main() {
	api := NewAPI()

	http.HandleFunc("/create-geozone", api.createGeozone)
	http.HandleFunc("/health", health)

	// static отдельно
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// root
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("API is running"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server started on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))

}
