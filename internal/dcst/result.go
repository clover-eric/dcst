package dcst

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func parseResultFile(path string) ([]IPResult, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, 0, err
	}
	if len(records) < 2 {
		return nil, max(0, len(records)-1), fmt.Errorf("测速结果为空")
	}

	results := make([]IPResult, 0, len(records)-1)
	seen := make(map[string]struct{}, len(records)-1)
	for _, row := range records[1:] {
		if len(row) < 7 {
			continue
		}
		ip := strings.TrimSpace(row[0])
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		sent, err1 := strconv.Atoi(strings.TrimSpace(row[1]))
		received, err2 := strconv.Atoi(strings.TrimSpace(row[2]))
		loss, err3 := strconv.ParseFloat(strings.TrimSpace(row[3]), 64)
		latency, err4 := strconv.ParseFloat(strings.TrimSpace(row[4]), 64)
		speed, err5 := strconv.ParseFloat(strings.TrimSpace(row[5]), 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			continue
		}
		seen[ip] = struct{}{}
		results = append(results, IPResult{
			IP:          ip,
			Sent:        sent,
			Received:    received,
			LossRate:    loss,
			LatencyMS:   latency,
			DownloadMBs: speed,
			Region:      strings.TrimSpace(row[6]),
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].DownloadMBs != results[j].DownloadMBs {
			return results[i].DownloadMBs > results[j].DownloadMBs
		}
		if results[i].LossRate != results[j].LossRate {
			return results[i].LossRate < results[j].LossRate
		}
		return results[i].LatencyMS < results[j].LatencyMS
	})
	if len(results) > ResultLimit {
		results = results[:ResultLimit]
	}
	if len(results) == 0 {
		return nil, len(records) - 1, fmt.Errorf("没有可用的测速结果")
	}
	return results, len(records) - 1, nil
}
