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

	//login    = "geozone"
	login    = "test_cola"
	password = "111111"

	//agentId = "947a60ea-3ccf-46c7-866a-42f6ab0ac793"
	agentId = "a2dcf51b-26d1-4c95-b9f3-292a2c099bd6"
)

type CreateRequest struct {
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Radius float64 `json:"radius"`
}

type UpdateRequest struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Radius float64 `json:"radius"`
}

type AuthResponse struct {
	AuthId string `json:"AuthId"`
	User   string `json:"User"`
}

type GeozoneItem struct {
	ID     int     `json:"id"`
	GUID   string  `json:"guid"`
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Radius float64 `json:"radius"`
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
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, PUT, OPTIONS")
}

// ======== Аутентификация ========
func (a *API) login() error {
	body := map[string]string{"login": login, "password": password}
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", loginURL, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	log.Printf("[LOGIN] Отправка запроса к %s", loginURL)
	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("[LOGIN] Ошибка запроса: %v", err)
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	log.Printf("[LOGIN] Статус ответа: %d, тело: %s", resp.StatusCode, string(data))

	var auth AuthResponse
	if err := json.Unmarshal(data, &auth); err != nil {
		log.Printf("[LOGIN] Ошибка парсинга JSON: %v", err)
		return err
	}
	if auth.AuthId == "" {
		log.Printf("[LOGIN] Пустой AuthId в ответе")
		return fmt.Errorf("empty auth id")
	}

	a.token = auth.AuthId
	u, _ := url.Parse(baseURL)
	a.client.Jar.SetCookies(u, []*http.Cookie{
		{Name: "ext_AuthId", Value: auth.AuthId, Path: "/"},
	})
	log.Printf("[LOGIN] ✅ Токен получен: %s", a.token)
	return nil
}

func (a *API) ensureAuth() error {
	if a.token == "" {
		log.Printf("[AUTH] Токен отсутствует, выполняем логин...")
		return a.login()
	}
	log.Printf("[AUTH] Токен уже есть: %s", a.token)
	return nil
}

// ======== Создание геозоны ========
func (a *API) createGeozone(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	w.Header().Set("Content-Type", "application/json")

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, 400)
		return
	}
	log.Printf("[CREATE] Запрос: name=%s, lat=%f, lon=%f, radius=%f", req.Name, req.Lat, req.Lon, req.Radius)

	if err := a.ensureAuth(); err != nil {
		log.Printf("[CREATE] Ошибка авторизации: %v", err)
		http.Error(w, `{"error":"auth failed"}`, 500)
		return
	}

	shapeObj := map[string]interface{}{
		"type":        "Point",
		"coordinates": []float64{req.Lon, req.Lat},
	}
	shapeBytes, _ := json.Marshal(shapeObj)

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
	b, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequest("POST", baseURL+"/api/gis/", bytes.NewBuffer(b))
	httpReq.Header.Set("Content-Type", "application/json;charset=utf-8")
	httpReq.Header.Set("X-Auth", a.token)
	httpReq.Header.Set("X-Agent", agentId)
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")

	log.Printf("[CREATE] Отправка запроса к %s", baseURL+"/api/gis/")
	resp, err := a.client.Do(httpReq)
	if err != nil {
		log.Printf("[CREATE] Ошибка: %v", err)
		http.Error(w, `{"error":"request failed"}`, 500)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[CREATE] Статус: %d, тело: %s", resp.StatusCode, string(body))
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// ======== Получение всех геозон ========
func (a *API) getGeozonesHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	w.Header().Set("Content-Type", "application/json")

	log.Printf("[GET] Начало обработки /geozones")
	if err := a.ensureAuth(); err != nil {
		log.Printf("[GET] Ошибка ensureAuth: %v", err)
		http.Error(w, `{"error":"auth failed"}`, 500)
		return
	}

	url := baseURL + "/api/gis"
	log.Printf("[GET] Запрашиваем URL: %s", url)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-Auth", a.token)
	req.Header.Set("X-Agent", agentId)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("[GET] Ошибка запроса: %v", err)
		http.Error(w, `{"error":"request failed"}`, 500)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[GET] Статус: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf(`{"error":"api returned %d"}`, resp.StatusCode), resp.StatusCode)
		return
	}

	var rawList []map[string]interface{}
	if err := json.Unmarshal(body, &rawList); err != nil {
		log.Printf("[GET] Ошибка парсинга массива: %v", err)
		http.Error(w, `{"error":"invalid json array"}`, 500)
		return
	}

	result := make([]GeozoneItem, 0, len(rawList))
	for _, item := range rawList {
		// ID
		var id int
		switch v := item["ID"].(type) {
		case float64:
			id = int(v)
		case int:
			id = v
		default:
			if v2, ok := item["id"]; ok {
				if f, ok := v2.(float64); ok {
					id = int(f)
				}
			}
		}
		// GUID
		guid := ""
		if v, ok := item["guid"].(string); ok {
			guid = v
		}
		// Name
		name := ""
		if v, ok := item["Name"].(string); ok {
			name = v
		} else if v, ok := item["name"].(string); ok {
			name = v
		}
		// Radius
		radius := 0.0
		if v, ok := item["Radius"].(float64); ok {
			radius = v
		} else if v, ok := item["radius"].(float64); ok {
			radius = v
		}
		// Shape
		var shapeStr string
		if v, ok := item["Shape"].(string); ok {
			shapeStr = v
		} else if v, ok := item["shape"].(string); ok {
			shapeStr = v
		}

		var lat, lon float64
		if shapeStr != "" {
			var shape map[string]interface{}
			if err := json.Unmarshal([]byte(shapeStr), &shape); err == nil {
				if coords, ok := shape["coordinates"].([]interface{}); ok && len(coords) == 2 {
					lon, _ = coords[0].(float64)
					lat, _ = coords[1].(float64)
				}
			}
		}

		result = append(result, GeozoneItem{
			ID:     id,
			GUID:   guid,
			Name:   name,
			Lat:    lat,
			Lon:    lon,
			Radius: radius,
		})
		log.Printf("[GET] Обработана геозона: id=%d, guid=%s, name=%s, lat=%f, lon=%f, radius=%f", id, guid, name, lat, lon, radius)
	}

	log.Printf("[GET] ✅ Успешно получено %d геозон", len(result))
	json.NewEncoder(w).Encode(result)
}

// ======== Обновление геозоны ========
func (a *API) updateGeozoneHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	w.Header().Set("Content-Type", "application/json")

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, 400)
		return
	}
	log.Printf("[UPDATE] Запрос: id=%d, name=%s, lat=%f, lon=%f, radius=%f", req.ID, req.Name, req.Lat, req.Lon, req.Radius)

	if req.ID == 0 {
		log.Printf("[UPDATE] Ошибка: id=0, невозможно обновить")
		http.Error(w, `{"error":"id is required"}`, 400)
		return
	}

	if err := a.ensureAuth(); err != nil {
		log.Printf("[UPDATE] Ошибка авторизации: %v", err)
		http.Error(w, `{"error":"auth failed"}`, 500)
		return
	}

	// 1. Получаем полные данные геозоны по ID
	getURL := fmt.Sprintf("%s/api/v3/gis/%d", baseURL, req.ID)
	log.Printf("[UPDATE] Запрашиваем полные данные геозоны: %s", getURL)

	getReq, _ := http.NewRequest("GET", getURL, nil)
	getReq.Header.Set("X-Auth", a.token)
	getReq.Header.Set("X-Agent", agentId)

	getResp, err := a.client.Do(getReq)
	if err != nil || getResp.StatusCode != http.StatusOK {
		log.Printf("[UPDATE] Ошибка получения данных геозоны: %v, статус: %d", err, getResp.StatusCode)
		http.Error(w, `{"error":"failed to fetch geozone data"}`, 500)
		return
	}
	defer getResp.Body.Close()

	getBody, _ := io.ReadAll(getResp.Body)
	log.Printf("[UPDATE] Получены данные геозоны: %s", string(getBody))

	var fullData map[string]interface{}
	if err := json.Unmarshal(getBody, &fullData); err != nil {
		log.Printf("[UPDATE] Ошибка парсинга данных геозоны: %v", err)
		http.Error(w, `{"error":"invalid geozone data"}`, 500)
		return
	}

	// 2. Обновляем необходимые поля
	fullData["name"] = req.Name
	fullData["radius"] = req.Radius
	fullData["shape_format"] = "geojson"

	shapeObj := map[string]interface{}{
		"type":        "Point",
		"coordinates": []float64{req.Lon, req.Lat},
	}
	shapeBytes, _ := json.Marshal(shapeObj)
	fullData["shape"] = string(shapeBytes)

	// Убеждаемся, что unitId установлен
	if _, ok := fullData["unitId"]; !ok {
		fullData["unitId"] = agentId
	}

	// 3. Отправляем PUT-запрос с полными данными
	updateBody, _ := json.Marshal(fullData)
	log.Printf("[UPDATE] Тело PUT-запроса: %s", string(updateBody))

	putReq, _ := http.NewRequest("PUT", baseURL+"/api/v3/gis", bytes.NewBuffer(updateBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("X-Auth", a.token)
	putReq.Header.Set("X-Agent", agentId)

	putResp, err := a.client.Do(putReq)
	if err != nil {
		log.Printf("[UPDATE] Ошибка PUT-запроса: %v", err)
		http.Error(w, `{"error":"update request failed"}`, 500)
		return
	}
	defer putResp.Body.Close()

	putBody, _ := io.ReadAll(putResp.Body)
	log.Printf("[UPDATE] Статус PUT: %d, тело: %s", putResp.StatusCode, string(putBody))

	w.WriteHeader(putResp.StatusCode)
	w.Write(putBody)
}

// ======== Health check ========
func health(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func main() {
	api := NewAPI()

	http.HandleFunc("/create-geozone", api.createGeozone)
	http.HandleFunc("/geozones", api.getGeozonesHandler)
	http.HandleFunc("/update-geozone", api.updateGeozoneHandler)
	http.HandleFunc("/health", health)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "./static/index.html")
			return
		}
		http.ServeFile(w, r, "./static"+r.URL.Path)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Сервер запущен на порту %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
