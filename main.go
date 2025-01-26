package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	_ "github.com/lib/pq"
)

// Database credentials and connection string
const (
	dbHost     = "194.5.195.119"
	dbPort     = 5432
	dbUser     = "mahdi"
	dbPassword = "mahdi"
	dbName     = "gps_data"
)

// MQTT credentials and topic
const (
	mqttHost     = "194.5.195.119"
	mqttPort     = 1883
	mqttUser     = "myuser"
	mqttPassword = "mypassword"
	topic        = "test-topic"
)

// Message handler for MQTT
func messageHandler(client mqtt.Client, msg mqtt.Message) {
	location, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		return
	}
	payload := string(msg.Payload())
	data := strings.Split(payload, ",")
	if len(data) < 5 {
		log.Printf("Invalid payload format: %s", payload)
		return
	}
	print(payload)

	hexValue4 := data[3]
	hexValue5 := data[4]

	// Convert the 4th value from hex to float64
	lat, err := hexToFloat64(hexValue4)
	if err != nil {
		log.Printf("Error converting 4th value (%s) to float64: %w", hexValue4, err)
		return
	}

	// Convert the 5th value from hex to float64
	lon, err := hexToFloat64(hexValue5)
	if err != nil {
		log.Printf("Error converting 5th value (%s) to float64: %v", hexValue5, err)
		return
	}

	// Insert data into PostgreSQL
	insertGPSData(lat, lon, location)
}

// Function to insert GPS data into PostgreSQL
func insertGPSData(latitude, longitude float64, loc *time.Location) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Insert GPS data into the gps_location table
	query := `INSERT INTO gps_location (user_id, latitude, longitude, timestamp) VALUES ($1, $2, $3, $4)`
	_, err = db.Exec(query, 0, latitude, longitude, time.Now().In(loc))
	if err != nil {
		log.Printf("Failed to insert data: %v", err)
		return
	}

	log.Printf("Inserted GPS data for user 0: (%f, %f)", latitude, longitude)
}

// Function to connect and subscribe to the MQTT topic
func connectAndSubscribe() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", mqttHost, mqttPort))
	opts.SetUsername(mqttUser)
	opts.SetPassword(mqttPassword)
	opts.SetDefaultPublishHandler(messageHandler)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", token.Error())
	}

	if token := client.Subscribe(topic, 1, nil); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to subscribe to topic %s: %v", topic, token.Error())
	}

	log.Printf("Subscribed to MQTT topic: %s", topic)
	select {} // Keep the connection alive
}

func hexToFloat64(hexStr string) (float64, error) {
	// Convert the hex string to a decimal integer
	decimalValue, err := strconv.ParseInt(hexStr, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid hex string: %w", err)
	}

	// Divide by 1,000,000 to get a float64 representation
	return float64(decimalValue) / 1000000, nil
}

func serveHTML(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}
func serveHTMLHistory(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "history.html")
}

// Function to get the latest GPS data
func getLastLocation(w http.ResponseWriter, r *http.Request) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		http.Error(w, "Failed to connect to database", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	var latitude, longitude float64
	query := `SELECT latitude, longitude FROM gps_location ORDER BY timestamp DESC LIMIT 1`
	err = db.QueryRow(query).Scan(&latitude, &longitude)
	if err != nil {
		http.Error(w, "Failed to fetch data", http.StatusInternalServerError)
		return
	}
	response := map[string]float64{
		"latitude":  latitude,
		"longitude": longitude,
	}

	// Encode the response as JSON
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func getLocationHistory(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for 'from' and 'to' timestamps
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	// Validate the timestamps
	if from == "" || to == "" {
		http.Error(w, "Both 'from' and 'to' query parameters are required", http.StatusBadRequest)
		return
	}

	// Parse timestamps
	fromTime, err := time.Parse(time.RFC3339, from)
	if err != nil {
		http.Error(w, "Invalid 'from' timestamp format", http.StatusBadRequest)
		return
	}

	toTime, err := time.Parse(time.RFC3339, to)
	if err != nil {
		http.Error(w, "Invalid 'to' timestamp format", http.StatusBadRequest)
		return
	}

	// Connect to the database
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		http.Error(w, "Failed to connect to database", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Query for GPS data within the specified time range
	query := `SELECT latitude, longitude, timestamp FROM gps_location WHERE timestamp BETWEEN $1 AND $2 ORDER BY timestamp ASC`
	rows, err := db.Query(query, fromTime, toTime)
	if err != nil {
		http.Error(w, "Failed to fetch data", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Collect the data in a JSON-friendly format
	var locations []map[string]interface{}
	for rows.Next() {
		var latitude, longitude float64
		var timestamp time.Time
		if err := rows.Scan(&latitude, &longitude, &timestamp); err != nil {
			http.Error(w, "Failed to parse data", http.StatusInternalServerError)
			return
		}
		locations = append(locations, map[string]interface{}{
			"latitude":  latitude,
			"longitude": longitude,
			"timestamp": timestamp,
		})
	}

	// Write the JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(locations); err != nil {
		http.Error(w, "Failed to encode data", http.StatusInternalServerError)
	}
}

func main() {
	// Set up the HTTP handler
	go connectAndSubscribe()
	http.HandleFunc("/", serveHTML)
	http.HandleFunc("/map/history", serveHTMLHistory)
	http.HandleFunc("/map/last-location", getLastLocation)
	http.HandleFunc("/map/location-history", getLocationHistory)

	// Serve static files (CSS, JS, etc.)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Start the server on port 8080
	fmt.Println("Server started at http://localhost:8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
