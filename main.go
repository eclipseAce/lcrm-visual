package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"

	rice "github.com/GeertJohan/go.rice"
	"github.com/gin-gonic/gin"

	"github.com/tarm/serial"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Serial struct {
		Port     string `yaml:"port"`
		BaudRate int    `yaml:"baud-rate"`
		DataBits byte   `yaml:"data-bits"`
		Parity   string `yaml:"parity"`
		StopBits byte   `yaml:"stop-bits"`
	} `yaml:"serial"`
	HTTP struct {
		BindAddress string `yaml:"bind-address"`
	} `yaml:"http"`
}

var (
	configFileName string
	config         Config
	engine         *gin.Engine
	box            *rice.HTTPBox
)

func main() {
	box = rice.MustFindBox("ui").HTTPBox()

	flag.StringVar(&configFileName, "c", "config.yml", "Config file name")
	flag.Parse()

	content, err := ioutil.ReadFile(configFileName)
	if err != nil {
		panic(err)
	}
	config = Config{}
	if err := yaml.Unmarshal(content, &config); err != nil {
		panic(err)
	}
	fmt.Println(config)

	port, err := serial.OpenPort(&serial.Config{
		Name:     config.Serial.Port,
		Baud:     config.Serial.BaudRate,
		Size:     config.Serial.DataBits,
		Parity:   serial.Parity([]byte(config.Serial.Parity)[0]),
		StopBits: serial.StopBits(config.Serial.StopBits),
	})
	if err != nil {
		panic(err)
	}
	defer port.Close()

	engine = gin.Default()
	api := engine.Group("/api")
	{
		api.GET("/samples", func(c *gin.Context) {
			var params struct {
				Prefix uint16
				Size   uint16 `form:"size"`
				RSel   uint8  `form:"rsel"`
				IGain  uint8  `form:"ig"`
				VGain  uint8  `form:"vg"`
			}
			if err := c.Bind(&params); err != nil {
				c.JSON(http.StatusOK, gin.H{"status": "fail", "reason": err})
				return
			}
			params.Prefix = 0xF1F2
			if err := binary.Write(port, binary.LittleEndian, params); err != nil {
				c.JSON(http.StatusOK, gin.H{"status": "fail", "reason": err})
				return
			}

			if err := waitForPrefix(port, 0xF1F2); err != nil {
				c.JSON(http.StatusOK, gin.H{"status": "fail", "reason": err})
				return
			}
			data := make([][]uint16, params.Size)
			for i := range data {
				vals := make([]uint16, 2)
				for j := range vals {
					if err := binary.Read(port, binary.LittleEndian, &vals[j]); err != nil {
						c.JSON(http.StatusOK, gin.H{"status": "fail", "reason": err})
					}
				}
				data[i] = vals
			}
			c.JSON(http.StatusOK, gin.H{"status": "ok", "data": data})
		})
	}
	engine.NoRoute(func(c *gin.Context) {
		c.FileFromFS(c.Request.RequestURI, box)
	})
	engine.Run(config.HTTP.BindAddress)

}

func waitForPrefix(port *serial.Port, prefix uint16) error {
	value := uint16(0)
	buf := make([]byte, 1)
	for value != prefix {
		if _, err := port.Read(buf); err != nil {
			return err
		}
		value = (value >> 8) | (uint16(buf[0]) << 8)
	}
	return nil
}
