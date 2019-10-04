package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/cpu"
)

func main() {
	location := os.Getenv("LOCATION")
	if location == "" {
		log.Println("set env var LOCATION=AUS-QLD to change the location")
		location = "AUS-QLD"
	}

	info, err := cpu.Info()
	if err != nil {
		log.Println(err)
		log.Fatal("unable to find cpu info")
	}

	wattPerThread := cpuTDP(info[0].ModelName) / float64(runtime.NumCPU())
	kiloWattHourPerThread := wattPerThread / 1000
	gramsOfCo2PerHourPerThread := kiloWattHourPerThread * carbonIntensity(location)
	gramsOfCo2PerThreadPerNS := gramsOfCo2PerHourPerThread / float64(time.Second/time.Nanosecond)

	c := exec.Command("go", os.Args[1:]...)
	stdOut, err := c.StdoutPipe()
	if err != nil {
		panic(err)
	}

	stdErr, err := c.StderrPipe()
	if err != nil {
		panic(err)
	}

	go func() {
		io.Copy(os.Stderr, stdErr)
	}()

	r := regexp.MustCompile(`Benchmark(.*)-([0-8]+)(\s*)([0-9]+)(\s*)([0-9]+) ns/op(.*)`)

	go func() {
		b := bufio.NewReader(stdOut)
		for {
			row, err := b.ReadString('\n')
			if err == io.EOF {
				return
			}
			if err != nil {
				panic(err)
			}

			if !r.MatchString(row) {
				fmt.Print(row)
				continue
			}

			x := r.FindStringSubmatch(row)

			nsPerOp, err := strconv.Atoi(x[6])
			if err != nil {
				panic(err)
			}

			gco2perop := float64(nsPerOp) * gramsOfCo2PerThreadPerNS

			fmt.Print(strings.TrimSpace(row))
			fmt.Printf("\t%.15f g CO2/op\n", gco2perop)
		}
	}()

	err = c.Run()
	if err != nil {
		panic(err)
	}
}

func carbonIntensity(location string) float64 {
	return loadFromData("co2.json", location)
}

func cpuTDP(model string) float64 {
	return loadFromData("tdp.json", model)
}

func loadFromData(filename, lookup string) float64 {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Println(err)
		log.Fatal("Can't locate files")
	}

	f, err := os.Open(filepath.Join(dir, filename))
	if err != nil {
		log.Println(err)
		log.Fatalf("Unable to read %s", filename)
	}
	defer f.Close()

	dataMap := map[string]float64{}
	if err := json.NewDecoder(f).Decode(&dataMap); err != nil {
		log.Println(err)
		log.Fatalf("unable to parse %s", filename)
	}

	value, found := dataMap[lookup]
	if !found {
		log.Fatalf("No info for %s", lookup)
	}

	return value
}
