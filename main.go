package main

import (
    "fmt"
	"io/ioutil"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
	"strconv"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/wonderivan/logger"
)

func GetCacheData(rate_debug_path string) (dirty float64, target float64) {
	content, err := ioutil.ReadFile(rate_debug_path)
    if err != nil {
        fmt.Println("File reading error", err)
        return
    }
    r := regexp.MustCompile(`dirty:\s*(\d+\.\d+)([kMG])\s*target:\s*(\d+\.\d+)([kMG])`)
    match := r.FindStringSubmatch(string(content))

	// match[1] dirty 脏数据的大小
	// match[2] dirty 脏数据的单位 k,M,G
	// match[3] target 总缓存的大小
	// match[4] target 总缓存的单位 G

	target_size := ToFloat(match[3])
	if match[2] == "M" {
		target_size = target_size * 1024
	} else if match[2] == "k" {
		target_size = target_size * 1024 * 1024
	}

    dirty = ToFloat(match[1])
    target = target_size
	logger.Info("The current bcache dirty data is %f, target data is %f", dirty, target)

	return dirty, target
}

func ToFloat(d_str string) (res float64) {
	res, err := strconv.ParseFloat(d_str, 64)
	if err != nil {
		panic(err)
	}
	return res
}

var (
	percent_50_rate_minimum string   // 下刷速率 2M
	percent_75_rate_minimum string   // 下刷速率 4M
	percent_90_rate_minimum string   // 下刷速率 8M
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "resize_bcache_minimum",
		Short: "resize bcache writeback rate minimum",
		Long:  `Dynamically resize the bcache writeback rate minumum`,
		Run: func(cmd *cobra.Command, args []string) {
			handler()
		},
	}
	rootCmd.PersistentFlags().StringVar(&percent_50_rate_minimum, "percent_50_rate_minimum", "4096", "The dirty data rate is greater than 50 less than 75 resize minimum resize 2M/s")
	rootCmd.PersistentFlags().StringVar(&percent_75_rate_minimum, "percent_75_rate_minimum", "8192", "The dirty data rate is greater than 75 less than 90 resize minimum resize 4M/s")
	rootCmd.PersistentFlags().StringVar(&percent_90_rate_minimum, "percent_90_rate_minimum", "16384", "The dirty data rate is greater than 90 resize 8M/s")

    if err := rootCmd.Execute(); err != nil {
        fmt.Println(err)
        os.Exit(1)
    }
}

func handler() {
	root := "/sys/block/"
    keyword := "bcache"
	rate_path_postfix := "/bcache/writeback_rate_debug"
	rate_minimum_path := "writeback_rate_minimum"

	var bcache_rate_path []string

    err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        if !info.IsDir() && strings.Contains(info.Name(), keyword) {
            rate_path := fmt.Sprintf(path + rate_path_postfix)
			bcache_rate_path = append(bcache_rate_path, rate_path)
        }
        return nil
    })
	if err != nil {
        fmt.Println(err)
    }

	// 获取 dirty, target 大小
	for _, bcache_rate := range bcache_rate_path {
		dirty, target := GetCacheData(bcache_rate)
		rate := int(dirty / target)
		bcache_writeback_rate_minimum := ""
		bcache_dir := filepath.Dir(bcache_rate)
		bcache_writeback_rate_minimum = bcache_dir + "/" + rate_minimum_path

		if rate < 50 {
			logger.Info("dirty data rate is less than 50.")
		} else if rate <= 75 {
			// 调整下刷速率为 2M
			command := fmt.Sprintf("echo" + " " + percent_50_rate_minimum + " > " + bcache_writeback_rate_minimum)
			cmd := exec.Command("sh", "-c", command)
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil {
				logger.Error("exec command error", stdoutStderr, err)
			}
			logger.Info("dirty data rate is greater than 50 less than 75, resize bcache writeback rate minimum 2M")
		} else if rate <= 90 {
			// 调整下刷速率为 4M
			command := fmt.Sprintf("echo" + " " + percent_75_rate_minimum + " > " + bcache_writeback_rate_minimum)
			cmd := exec.Command("sh", "-c", command)
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil {
				logger.Error("exec command error", stdoutStderr, err)
			}
			logger.Info("dirty data rate is greater than 50 less than 75, resize bcache writeback rate minimum 4M")
		} else if rate > 90 {
			// 调整下刷速率为 8M
			command := fmt.Sprintf("echo" + " " + percent_90_rate_minimum + " > " + bcache_writeback_rate_minimum)
			cmd := exec.Command("sh", "-c", command)
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil {
				logger.Error("exec command error", stdoutStderr, err)
			}
			logger.Info("dirty data rate is greater than 50 less than 75, resize bcache writeback rate minimum 8M")
		}
	}
}
