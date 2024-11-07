package main

import (
	"fmt"

	"github.com/oarkflow/json"
)

type Config struct {
	Name          string `json:"name"`
	Key           string `json:"key"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Driver        string `json:"driver"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Database      string `json:"database"`
	SslMode       string `json:"ssl_mode"`
	Timezone      string `json:"timezone"`
	Charset       string `json:"charset"`
	Location      string `json:"location"`
	DisableLogger bool   `json:"disable_logger"`
	MaxLifetime   int64  `yaml:"max_lifetime" json:"max_lifetime"`
	MaxIdleTime   int64  `yaml:"max_idle_time" json:"max_idle_time"`
	MaxOpenCons   int    `yaml:"max_open_cons" json:"max_open_cons"`
	MaxIdleCons   int    `yaml:"max_idle_cons" json:"max_idle_cons"`
}

func main() {
	// JSON where power is an int and level is a string
	jsonData1 := `[{"charset":"utf8mb4","database":"clear_dev","driver":"postgresql","host":"127.0.0.1","key":"clear","name":"clear","options":{},"password":"postgres","port":"5432","ssl_mode":"disable","timezone":"UTC","username":"postgres"},{"charset":"utf8mb4","database":"eamitest","driver":"mysql","host":"127.0.0.1","key":"mirth","name":"mirth","options":{},"password":"root","port":3306,"ssl_mode":"disable","timezone":"UTC","username":"root"},{"charset":"utf8mb4","database":"clear20_dev","driver":"postgresql","host":"127.0.0.1","key":"cleardb","name":"CleardB","options":{},"password":"postgres","port":"5432","ssl_mode":"disable","timezone":"UTC","username":"postgres"}]`
	jsonData2 := `{"charset":"utf8mb4","database":"clear_dev","driver":"postgresql","host":"127.0.0.1","key":"clear","name":"clear","options":{},"password":"postgres","port":"5432","ssl_mode":"disable","timezone":"UTC","username":"postgres"}`

	var opts1 []Config

	// Unmarshal both JSON examples using the generic unmarshal function
	if err := json.FixAndUnmarshal([]byte(jsonData1), &opts1); err != nil {
		fmt.Printf("Error unmarshaling opts1: %v\n", err)
		return
	}
	fmt.Printf("Unmarshaled opts1: %+v\n", opts1)

	var opts2 Config

	// Unmarshal both JSON examples using the generic unmarshal function
	if err := json.FixAndUnmarshal([]byte(jsonData2), &opts2); err != nil {
		fmt.Printf("Error unmarshaling opts1: %v\n", err)
		return
	}
	fmt.Printf("Unmarshaled opts1: %+v\n", opts2)

}
