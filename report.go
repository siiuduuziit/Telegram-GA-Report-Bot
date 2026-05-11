package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
)

type metricSet struct {
	Users          int64
	NewUsers       int64
	Sessions       int64
	BounceRate     float64
	AvgSessionSecs float64
}

type reportBlock struct {
	Title   string
	Current metricSet
	Prev    metricSet
}

func buildReport(ctx context.Context, gaSvc *analyticsdata.Service, propertyID string) string {
	currentRange := &analyticsdata.DateRange{StartDate: "30daysAgo", EndDate: "today"}
	previousRange := &analyticsdata.DateRange{StartDate: "60daysAgo", EndDate: "31daysAgo"}

	totalCurrent, err := fetchAggregate(ctx, gaSvc, propertyID, currentRange)
	if err != nil {
		return fmt.Sprintf("Lỗi lấy dữ liệu hiện tại: %v", err)
	}

	totalPrev, err := fetchAggregate(ctx, gaSvc, propertyID, previousRange)
	if err != nil {
		return fmt.Sprintf("Lỗi lấy dữ liệu total kỳ trước: %v", err)
	}

	channelCurrent, err := fetchByChannel(ctx, gaSvc, propertyID, currentRange)
	if err != nil {
		return fmt.Sprintf("Lỗi lấy dữ liệu channel hiện tại: %v", err)
	}
	channelPrev, err := fetchByChannel(ctx, gaSvc, propertyID, previousRange)
	if err != nil {
		return fmt.Sprintf("Lỗi lấy dữ liệu channel kỳ trước: %v", err)
	}

	channels := mergeChannels(channelCurrent, channelPrev)
	sort.Slice(channels, func(i, j int) bool { return channels[i].Current.Sessions > channels[j].Current.Sessions })

	lines := []string{
		fmt.Sprintf("Báo cáo vnetwork.vn (%s)", time.Now().Format("02-01-2006 15:04")),
		"So sánh: 30 ngày gần nhất vs 30 ngày trước đó",
		"",
		formatBlock(1, reportBlock{Title: "Total Traffic", Current: totalCurrent, Prev: totalPrev}),
	}

	for i, block := range channels {
		if i >= 4 {
			break
		}
		lines = append(lines, "", formatBlock(i+2, block))
	}

	return strings.Join(lines, "\n")
}

func fetchAggregate(ctx context.Context, gaSvc *analyticsdata.Service, propertyID string, dr *analyticsdata.DateRange) (metricSet, error) {
	resp, err := gaSvc.Properties.RunReport("properties/"+propertyID, &analyticsdata.RunReportRequest{
		DateRanges: []*analyticsdata.DateRange{dr},
		Metrics: []*analyticsdata.Metric{
			{Name: "totalUsers"},
			{Name: "newUsers"},
			{Name: "sessions"},
			{Name: "bounceRate"},
			{Name: "averageSessionDuration"},
		},
	}).Context(ctx).Do()
	if err != nil {
		return metricSet{}, err
	}

	if len(resp.Rows) == 0 || len(resp.Rows[0].MetricValues) < 5 {
		return metricSet{}, nil
	}

	m := resp.Rows[0].MetricValues
	return metricSet{
		Users:          parseInt(m[0].Value),
		NewUsers:       parseInt(m[1].Value),
		Sessions:       parseInt(m[2].Value),
		BounceRate:     parseFloat(m[3].Value) * 100,
		AvgSessionSecs: parseFloat(m[4].Value),
	}, nil
}

func fetchByChannel(ctx context.Context, gaSvc *analyticsdata.Service, propertyID string, dr *analyticsdata.DateRange) (map[string]metricSet, error) {
	resp, err := gaSvc.Properties.RunReport("properties/"+propertyID, &analyticsdata.RunReportRequest{
		DateRanges: []*analyticsdata.DateRange{dr},
		Dimensions: []*analyticsdata.Dimension{{Name: "sessionDefaultChannelGroup"}},
		Metrics: []*analyticsdata.Metric{
			{Name: "totalUsers"},
			{Name: "newUsers"},
			{Name: "sessions"},
			{Name: "bounceRate"},
			{Name: "averageSessionDuration"},
		},
		OrderBys: []*analyticsdata.OrderBy{{
			Metric: &analyticsdata.MetricOrderBy{MetricName: "sessions"},
			Desc:   true,
		}},
		Limit: 20,
	}).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	out := map[string]metricSet{}
	for _, row := range resp.Rows {
		if len(row.DimensionValues) == 0 || len(row.MetricValues) < 5 {
			continue
		}
		ch := row.DimensionValues[0].Value
		m := row.MetricValues
		out[ch] = metricSet{
			Users:          parseInt(m[0].Value),
			NewUsers:       parseInt(m[1].Value),
			Sessions:       parseInt(m[2].Value),
			BounceRate:     parseFloat(m[3].Value) * 100,
			AvgSessionSecs: parseFloat(m[4].Value),
		}
	}

	return out, nil
}

func mergeChannels(current map[string]metricSet, prev map[string]metricSet) []reportBlock {
	all := map[string]bool{}
	for k := range current {
		all[k] = true
	}
	for k := range prev {
		all[k] = true
	}

	blocks := make([]reportBlock, 0, len(all))
	for channel := range all {
		blocks = append(blocks, reportBlock{
			Title:   normalizeChannelName(channel),
			Current: current[channel],
			Prev:    prev[channel],
		})
	}
	return blocks
}

func formatBlock(index int, b reportBlock) string {
	return strings.Join([]string{
		fmt.Sprintf("%d. %s", index, b.Title),
		fmt.Sprintf("- Users:		 %d / %d (%s)", b.Current.Users, b.Prev.Users, trendText(float64(b.Current.Users), float64(b.Prev.Users), false)),
		fmt.Sprintf("- New User: 	 %d / %d (%s)", b.Current.NewUsers, b.Prev.NewUsers, trendText(float64(b.Current.NewUsers), float64(b.Prev.NewUsers), false)),
		fmt.Sprintf("- Sessions: 	 %d / %d (%s)", b.Current.Sessions, b.Prev.Sessions, trendText(float64(b.Current.Sessions), float64(b.Prev.Sessions), false)),
		fmt.Sprintf("- Bounce rate:  %.2f%% / %.2f%% (%s)", b.Current.BounceRate, b.Prev.BounceRate, trendText(b.Current.BounceRate, b.Prev.BounceRate, true)),
		fmt.Sprintf("- Time on site: %s / %s (%s)", formatDuration(b.Current.AvgSessionSecs), formatDuration(b.Prev.AvgSessionSecs), trendText(b.Current.AvgSessionSecs, b.Prev.AvgSessionSecs, false)),
	}, "\n")
}

func trendText(current, previous float64, invert bool) string {
	if previous == 0 {
		if current == 0 {
			return "không đổi"
		}
		if invert {
			return "tốt hơn (không so sánh %)"
		}
		return "tăng (không so sánh %)"
	}

	change := ((current - previous) / previous) * 100
	if invert {
		if change < 0 {
			return fmt.Sprintf("giảm %.2f%%", math.Abs(change))
		}
		if change > 0 {
			return fmt.Sprintf("xấu đi %.2f%%", change)
		}
		return "không đổi"
	}

	if change > 0 {
		return fmt.Sprintf("tăng %.2f%%", change)
	}
	if change < 0 {
		return fmt.Sprintf("giảm %.2f%%", math.Abs(change))
	}
	return "không đổi"
}

func formatDuration(seconds float64) string {
	total := int(seconds + 0.5)
	m := total / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

func normalizeChannelName(ch string) string {
	switch strings.ToLower(ch) {
	case "direct":
		return "Direct Traffic"
	case "organic search":
		return "Organic Search Traffic"
	case "unassigned":
		return "Unassigned Traffic"
	default:
		return ch + " Traffic"
	}
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(strings.Split(s, ".")[0], 10, 64)
	return n
}

func parseFloat(s string) float64 {
	n, _ := strconv.ParseFloat(s, 64)
	return n
}
