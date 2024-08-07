package co2client

import (
	"bufio"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	"github.com/hn-11/atmo-client/internal/pkg/db"
	"github.com/tarm/serial"
)

const (
	DEV              = "/dev/ttyACM0"
	BUCKET_NAME      = "CO2"
	MEASUREMENT_NAME = "co2"
)

type values struct {
	co2 float64
	hum float64
	tmp float64
}

type Client struct {
	re       *regexp.Regexp
	conn     *serial.Port
	reader   *bufio.Reader
	dbClient db.Client
}

func correct(raw values) (corrected *values) {
	// https://x.com/rynan4818/status/1627089985454366720
	tmp := raw.tmp - 4.5
	hum := raw.hum * raw.tmp * (tmp + 237.3) / (tmp * (raw.tmp + 237.3))
	return &values{raw.co2, hum, tmp}
}

func Init() (*Client, error) {
	conn, err := serial.OpenPort(&serial.Config{Name: DEV, Baud: 115200})
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`CO2=(?P<co2>\d+),HUM=(?P<hum>\d+\.\d+),TMP=(?P<tmp>-?\d+\.\d+)`)

	_, err = conn.Write([]byte("STA\r\n"))
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(conn)

	dbClient := db.Client{Bucket: BUCKET_NAME, Measurement: MEASUREMENT_NAME}
	dbClient.Connect()

	return &Client{re, conn, reader, dbClient}, nil
}

func (c *Client) Close() error {
	_, err := c.conn.Write([]byte("STP\r\n"))
	if err != nil {
		return err
	}

	err = c.conn.Close()
	return err
}

func (c *Client) Start() error {
	for {
		line, _, err := c.reader.ReadLine()
		if err != nil {
			return err
		}

		v, err := c.read(string(line))
		if err != nil {
			log.Print(err)
		}
		if v != nil {
			c.write(correct(*v))
		}
	}
}

func (c *Client) read(line string) (value *values, err error) {
	if line == "OK STA" {
		return nil, nil
	}
	matches := c.re.FindStringSubmatch(line)
	if len(matches) != 4 {
		return nil, fmt.Errorf("invalid format: '%s'", line)
	}

	var result [3]float64
	for i, m := range matches[1:4] {
		result[i], err = strconv.ParseFloat(m, 64)
		if err != nil {
			return nil, err
		}
	}

	return &values{result[0], result[1], result[2]}, nil
}

func (c *Client) write(v *values) {
	c.dbClient.Write(map[string]interface{}{
		"CO2": v.co2,
		"HUM": v.hum,
		"TMP": v.tmp,
	}, time.Now())
}
