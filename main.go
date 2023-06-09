package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
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
	percent_50_rate_minimum string // 下刷速率 2M
	percent_75_rate_minimum string // 下刷速率 4M
	percent_90_rate_minimum string // 下刷速率 8M
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
	for {
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

		for _, bcache_rate := range bcache_rate_path {
			// 获取 bcache 设备名称
			bcache_disk := ""
			expr := `(?<=/sys/block/)[^/]+`
			reg, _ := regexp2.Compile(expr, 0)
			m, _ := reg.FindStringMatch(bcache_rate)
			if m != nil {
				bcache_disk = m.String()
			}

			// 获取 dirty, target 大小
			dirty, target := GetCacheData(bcache_rate)
			// 获取当前 dirty 脏数据写入的比例
			rate := int(dirty / target * 100)
			logger.Info("The bcache disk %s dirty data rate is %d", bcache_disk, rate)

			// 获取 bcache writeback_rate_minimum 路径
			bcache_writeback_rate_minimum := ""
			bcache_dir := filepath.Dir(bcache_rate)
			bcache_writeback_rate_minimum = bcache_dir + "/" + rate_minimum_path

			// 查看当前 writeback_rate_minimum 值
			content, err := ioutil.ReadFile(bcache_writeback_rate_minimum)
			if err != nil {
				fmt.Println("File reading error", err)
				return
			}
			file_rate_minimum, _ := strconv.Atoi(strings.Trim(string(content), "\n"))
			logger.Info("The bcache disk %s current writeback rate minimum is %d", bcache_disk, file_rate_minimum)

			if rate < 50 {
				// 判断 writeback_rate_minimum 值是否被修改
				if file_rate_minimum == 2048 {
					logger.Info("The bcache disk %s dirty data rate is less than 50.", bcache_disk)
					continue
				}
				// 将 writeback_rate_minimum 修改为初始值
				command := fmt.Sprintf("echo" + " " + "2048" + " > " + bcache_writeback_rate_minimum)
				cmd := exec.Command("sh", "-c", command)
				stdoutStderr, err := cmd.CombinedOutput()
				if err != nil {
					logger.Error("exec command error", stdoutStderr, err)
				}
				logger.Info("The bcache disk %s dirty data rate is less than 50.", bcache_disk)
			} else if rate > 50 && rate <= 75 {
				// 调整下刷速率为 2M
				percent_50_minimum, _ := strconv.Atoi(percent_50_rate_minimum)
				if file_rate_minimum == percent_50_minimum {
					continue
				}
				command := fmt.Sprintf("echo" + " " + percent_50_rate_minimum + " > " + bcache_writeback_rate_minimum)
				cmd := exec.Command("sh", "-c", command)
				stdoutStderr, err := cmd.CombinedOutput()
				if err != nil {
					logger.Error("exec command error", stdoutStderr, err)
				}
				logger.Info("The bcache disk %s dirty data rate is greater than 50 less than 75, resize bcache writeback rate minimum 2M", bcache_disk)
			} else if rate > 75 && rate <= 90 {
				// 调整下刷速率为 4M
				percent_75_minimum, _ := strconv.Atoi(percent_75_rate_minimum)
				if file_rate_minimum == percent_75_minimum {
					continue
				}
				command := fmt.Sprintf("echo" + " " + percent_75_rate_minimum + " > " + bcache_writeback_rate_minimum)
				cmd := exec.Command("sh", "-c", command)
				stdoutStderr, err := cmd.CombinedOutput()
				if err != nil {
					logger.Error("exec command error", stdoutStderr, err)
				}
				logger.Info("The bcache disk %s dirty data rate is greater than 50 less than 75, resize bcache writeback rate minimum 4M", bcache_disk)
			} else if rate > 90 {
				// 调整下刷速率为 8M
				percent_90_minimum, _ := strconv.Atoi(percent_90_rate_minimum)
				if file_rate_minimum == percent_90_minimum {
					continue
				}
				command := fmt.Sprintf("echo" + " " + percent_90_rate_minimum + " > " + bcache_writeback_rate_minimum)
				cmd := exec.Command("sh", "-c", command)
				stdoutStderr, err := cmd.CombinedOutput()
				if err != nil {
					logger.Error("exec command error", stdoutStderr, err)
				}
				logger.Info("The bcache disk %s dirty data rate is greater than 50 less than 75, resize bcache writeback rate minimum 8M", bcache_disk)
			}
		}
		time.Sleep(time.Minute)
	}
}
