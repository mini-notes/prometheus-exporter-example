package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	go_ora "github.com/sijms/go-ora/v2"
)

type ServerDetail struct {
	// Host    string
	Status  int
	Restime int
}

type DB_Status int

const (
	Up DB_Status = iota
	Down
)

var (
	Host   = "Host"
	Status = "Status"
)

// type metrics struct {
// 	taketime prometheus.Gauge
// 	status   prometheus.Gauge
// }

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
	// "server":   getenv_with_fallback("DB_MONITOR_SERVER", "localhost"),
	"port":     getenv_with_fallback("DB_MONITOR_PORT", "1521"),
	"password": getenv_with_fallback("DB_MONITOR_PASSWORD", "My1passw"),
}

func handleError(msg string, err error) {
	if err != nil {
		fmt.Println(msg, err)
		os.Exit(1)
	}
}

func doDB_query(dbParams map[string]string, query string, host string) int {
	port, err := strconv.Atoi(dbParams["port"])
	var db_status int = int(Up)

	handleError("Error during conversion", err)
	// set connection time for 10 second
	urlOptions := map[string]string{
		"CONNECTION TIMEOUT": "10",
	}
	databaseUrl := go_ora.BuildUrl(host, port, dbParams["service"], dbParams["username"], dbParams["password"], urlOptions)
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
	fmt.Printf("PING to server: %s - port: %d ", host, port)
	err = db.Ping()
	if err != nil {
		db_status = int(Down)
		fmt.Printf("error pinging db: %s", err)
	} else {
		db_status = int(Up)
		db_Select_query(db, query)
	}
	return db_status
}

func db_Select_query(db *sql.DB, query string) {

	// fmt.Println("db query:", query)

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

func pullMetrics(ctx context.Context, query string, host string) ServerDetail {

	var take_time int
	var server ServerDetail
	t := time.Now()
	s := doDB_query(localDB, query, host)
	take_time = int(time.Now().Sub(t).Milliseconds())

	// if server down -> take_time=0
	if s == int(Down) {
		take_time = 0
	}
	fmt.Printf("Time Elapsed: %d ms\n", take_time)
	server.Restime = take_time
	server.Status = s
	return server
}

func main() {
	reg := prometheus.NewRegistry()
	// hosts := "127.0.0.1;localhost"
	hosts := getenv_with_fallback("DB_MONITOR_SERVER", "localhost")
	hostList := strings.Split(hosts, ";")

	serverTakeTtime := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "Db_take_time",
		Help: "Time Elapsed query oracle db (ms)",
	}, []string{Host})

	serverStatus := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "db_status",
		Help: "Check database status: 0 - up, 1: down",
	}, []string{Host})

	reg.MustRegister(serverTakeTtime, serverStatus)
	var query string = getenv_with_fallback("DB_MONITOR_QUERY", "select employee_name,city from employees")
	pull_interval_duration, err := strconv.Atoi(getenv_with_fallback("DB_MONITOR_PULL_INTERVAL", "5"))
	handleError("Error during conversion", err)
	var listen_server string = getenv_with_fallback("LISTEN_SERVER", ":8081")

	//ctx
	ctx := context.Background()

	// init
	for _, host := range hostList {
		serverMetric := pullMetrics(ctx, query, host)
		serverTakeTtime.With(prometheus.Labels{
			Host: host,
		}).Set(float64(serverMetric.Restime))
		serverStatus.With(prometheus.Labels{
			Host: host,
		}).Set(float64(serverMetric.Status))
	}
	go func(ctx context.Context) {
		ticker := time.NewTicker(time.Duration(pull_interval_duration) * time.Second)
		for {
			select {
			case <-ctx.Done():
				fmt.Printf("Context done, stop consume metric for source")
				return
			case tm := <-ticker.C:
				fmt.Println("The Current time is: ", tm)
				for _, host := range hostList {
					serverMetric := pullMetrics(ctx, query, host)
					serverTakeTtime.With(prometheus.Labels{
						Host: host,
					}).Set(float64(serverMetric.Restime))
					serverStatus.With(prometheus.Labels{
						Host: host,
					}).Set(float64(serverMetric.Status))
				}
				fmt.Println("End Current time is: ", tm)
			}
		}
	}(ctx)

	promHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	http.Handle("/metrics", promHandler)
	// http.Handle("/metrics_default", promhttp.Handler())
	http.ListenAndServe(listen_server, nil)
}
