package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	go_ora "github.com/sijms/go-ora/v2"
)

type DB_Status int

const (
	Up DB_Status = iota
	Down
)

type metrics struct {
	taketime prometheus.Gauge
	status   prometheus.Gauge
}

func getenv_with_fallback(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

var localDB = map[string]string{
	"service":  getenv_with_fallback("DB_MONITOR_SERVICE", "XEPDB1"),
	"username": getenv_with_fallback("DB_MONITOR_USERNAME", "dzung"),
	"server":   getenv_with_fallback("DB_MONITOR_SERVER", "localhost"),
	"port":     getenv_with_fallback("DB_MONITOR_PORT", "1521"),
	"password": getenv_with_fallback("DB_MONITOR_PASSWORD", "My1passw"),
}

func handleError(msg string, err error) {
	if err != nil {
		fmt.Println(msg, err)
		os.Exit(1)
	}
}

func doDB_query(dbParams map[string]string, query string, metric metrics) {
	// connectionString := "oracle://" + dbParams["username"] + ":" + dbParams["password"] + "@" + dbParams["server"] + ":" + dbParams["port"] + "/" + dbParams["service"]

	// fmt.Printf("connectionString : %s\n", connectionString)
	port, err := strconv.Atoi(dbParams["port"])
	var db_status int = int(Up)

	handleError("Error during conversion", err)
	// set connection time for 10 second
	urlOptions := map[string]string{
		"CONNECTION TIMEOUT": "10",
	}
	databaseUrl := go_ora.BuildUrl(dbParams["server"], port, dbParams["service"], dbParams["username"], dbParams["password"], urlOptions)
	// fmt.Printf("connectionString : %s\n", databaseUrl)
	db, err := sql.Open("oracle", databaseUrl)

	if err != nil {
		panic(fmt.Errorf("error in sql.Open: %w", err))
	}
	// check hadle err
	defer func() {
		err = db.Close()
		if err != nil {
			fmt.Println("Can't close connection: ", err)
		}
	}()
	fmt.Printf("PING to server: %s - port: %d ", dbParams["server"], port)
	err = db.Ping()
	if err != nil {
		// panic(fmt.Errorf("error pinging db: %w", err))
		db_status = int(Down)
		fmt.Printf("error pinging db: %s", err)
	} else {
		db_status = int(Up)
		db_Select_query(db, query)
	}
	metric.status.Set(float64(db_status))
}

func db_Select_query(db *sql.DB, query string) {

	fmt.Println("db query:", query)

	// fetching multiple rows
	theRows, err := db.Query(query)
	handleError("Query for multiple rows", err)
	// closing the parent rows will automatically close cursor
	defer theRows.Close()
	var (
		first_prop  string
		second_prop string
	)
	for theRows.Next() {
		err := theRows.Scan(&first_prop, &second_prop)
		handleError("next row in multiple rows", err)
		fmt.Printf("The first propertie is: %s and second propertie is: %s \n", first_prop, second_prop)
	}
	err = theRows.Err()
	handleError("next row in multiple rows", err)

}

func pullMetrics(ctx context.Context, metric metrics, query string) error {
	var take_time int
	// fmt.Println("Local Database, simple connect string ")
	t := time.Now()
	doDB_query(localDB, query, metric)
	take_time = int(time.Now().Sub(t).Milliseconds())
	fmt.Printf("Time Elapsed: %d ms\n", take_time)
	metric.taketime.Set(float64(take_time))
	return nil
}
func NewMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		taketime: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "db_query",
			Name:      "taketime",
			Help:      "Time Elapsed query oracle db (ms)",
		}),
		status: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "db_check",
			Name:      "status",
			Help:      "Check database status: 0 - up, 1: down",
		}),
	}
	reg.MustRegister(m.taketime, m.status)
	return m
}

func main() {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	var query string = getenv_with_fallback("DB_MONITOR_QUERY", "select employee_name,city from employees")
	pull_interval_duration, err := strconv.Atoi(getenv_with_fallback("DB_MONITOR_PULL_INTERVAL", "5"))
	var listen_server string = getenv_with_fallback("LISTEN_SERVER", ":8081")

	handleError("Error during conversion", err)

	ctx := context.Background()
	go func(ctx context.Context) {
		ticker := time.NewTicker(time.Duration(pull_interval_duration) * time.Second)
		for {
			select {
			case <-ctx.Done():
				fmt.Printf("Context done, stop consume metric for source")
				return
			case _ = <-ticker.C:
				// fmt.Println("The Current time is: ", tm)
				pullMetrics(ctx, *m, query)
			}
		}
	}(ctx)

	promHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	http.Handle("/metrics", promHandler)
	// http.Handle("/metrics_default", promhttp.Handler())
	http.ListenAndServe(listen_server, nil)
}
