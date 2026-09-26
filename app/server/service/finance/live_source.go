package finance

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
)

type LiveSource struct {
	Now func() time.Time
}

func NewLiveSource() *LiveSource {
	return &LiveSource{Now: func() time.Time { return time.Now().UTC() }}
}

func (s *LiveSource) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *LiveSource) resolveInstrument(id string) (ListedInstrument, error) {
	inst, ok := LookupInstrument(id)
	if ok && inst.ID != InstrumentDemo {
		return inst, nil
	}
	_ = EnsureLiveCatalog(context.Background())
	inst, ok = LookupInstrument(id)
	if !ok || inst.ID == InstrumentDemo {
		return ListedInstrument{}, NewError(422, "validation", "UNSUPPORTED_INSTRUMENT", "标的不在覆盖目录")
	}
	return inst, nil
}

func (s *LiveSource) Financials(ctx context.Context, run RunSnapshot, metrics, periods []string) ([]ProviderRecord, int, error) {
	inst, err := s.resolveInstrument(run.InstrumentID)
	if err != nil {
		return nil, 0, err
	}
	if err := validateFinancialArgs(metrics, periods); err != nil {
		return nil, 0, err
	}
	httpc := NewCountedHTTP(FinancialHTTPLimit, ToolTimeout)
	if inst.Market == MarketHK {
		recs, err := s.hkFinancials(ctx, httpc, run, inst, metrics, periods)
		return recs, httpc.Count(), err
	}
	recs, err := s.aShareFinancials(ctx, httpc, run, inst, metrics, periods)
	return recs, httpc.Count(), err
}

func (s *LiveSource) aShareFinancials(ctx context.Context, httpc *CountedHTTP, run RunSnapshot, inst ListedInstrument, metrics, periods []string) ([]ProviderRecord, error) {
	need := sheetsNeeded(metrics)
	sheets := map[statementKind][]map[string]any{}
	var err error
	if _, ok := need[sheetIncome]; ok {
		sheets[sheetIncome], err = fetchEastMoneySheet(ctx, httpc, emIncome, inst.Symbol)
		if err != nil {
			return nil, err
		}
	}
	if _, ok := need[sheetBalance]; ok {
		sheets[sheetBalance], err = fetchEastMoneySheet(ctx, httpc, emBalance, inst.Symbol)
		if err != nil {
			return nil, err
		}
	}
	if _, ok := need[sheetCash]; ok {
		sheets[sheetCash], err = fetchEastMoneySheet(ctx, httpc, emCash, inst.Symbol)
		if err != nil {
			return nil, err
		}
	}
	retrieved := s.now()
	var recs []ProviderRecord
	for _, period := range periods {
		end := normalizePeriodEnd(period)
		start := strings.TrimSuffix(end, "-12-31") + "-01-01"
		var ms []Metric
		var originals []string
		notice := time.Time{}
		for _, name := range metrics {
			def, ok := metricDef(name)
			if !ok {
				return nil, NewError(400, "validation", "UNSUPPORTED_METRIC", "指标不在冻结目录")
			}
			row := annualRow(sheets[def.Sheet], end)
			if row == nil {
				return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "缺少已披露年度"+sheetLabel(def.Sheet))
			}
			if d, ok := parseEMDate(row["NOTICE_DATE"]); ok && notice.IsZero() {
				notice = d
			}
			val, ok := decimalYuan(row[def.AField])
			if !ok {
				return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "缺字段不得填0："+name)
			}
			ms = append(ms, Metric{Metric: name, PeriodStart: start, PeriodEnd: end, Value: val, Unit: "CNY", ValueType: "actual"})
			originals = append(originals, name+"="+val)
		}
		if notice.IsZero() {
			return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "披露日期不明，不能登记为事实证据")
		}
		published, available := conservativeDayTimes(notice)
		if published.After(run.AsOf) || available.After(run.AsOf) {
			return nil, NewError(400, "validation", "FUTURE_EVIDENCE", "晚于 as_of 的材料排除")
		}
		recs = append(recs, financialRecord(inst, end, "CNY", originals, ms, published, available, retrieved))
	}
	return recs, nil
}

func (s *LiveSource) hkFinancials(ctx context.Context, httpc *CountedHTTP, run RunSnapshot, inst ListedInstrument, metrics, periods []string) ([]ProviderRecord, error) {
	meta, err := fetchHKReportMeta(ctx, httpc, inst.Symbol)
	if err != nil {
		return nil, err
	}
	dates, err := matchHKAnnualDates(meta, periods)
	if err != nil {
		return nil, err
	}
	need := sheetsNeeded(metrics)
	sheets := map[statementKind][]map[string]any{}
	if _, ok := need[sheetIncome]; ok {
		sheets[sheetIncome], err = fetchHKSheet(ctx, httpc, hkIncome, inst.Symbol, dates)
		if err != nil {
			return nil, err
		}
	}
	if _, ok := need[sheetBalance]; ok {
		sheets[sheetBalance], err = fetchHKSheet(ctx, httpc, hkBalance, inst.Symbol, dates)
		if err != nil {
			return nil, err
		}
	}
	if _, ok := need[sheetCash]; ok {
		sheets[sheetCash], err = fetchHKSheet(ctx, httpc, hkCash, inst.Symbol, dates)
		if err != nil {
			return nil, err
		}
	}
	noticeHits, _ := searchCninfo(ctx, httpc, inst, "年报", 5)
	retrieved := s.now()
	var recs []ProviderRecord
	for _, period := range periods {
		end := normalizePeriodEnd(period)
		start := strings.TrimSuffix(end, "-12-31") + "-01-01"
		rowMeta := meta.ByPeriod[end]
		unit := "HKD"
		if rowMeta != nil {
			unit = mapHKCurrency(strVal(rowMeta["CURRENCY"]))
		}
		var ms []Metric
		var originals []string
		for _, name := range metrics {
			def, ok := metricDef(name)
			if !ok {
				return nil, NewError(400, "validation", "UNSUPPORTED_METRIC", "指标不在冻结目录")
			}
			val, ok := hkAmount(sheets[def.Sheet], end, def)
			if !ok {
				return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "缺字段不得填0："+name)
			}
			ms = append(ms, Metric{Metric: name, PeriodStart: start, PeriodEnd: end, Value: val, Unit: unit, ValueType: "actual"})
			originals = append(originals, name+"="+val)
		}
		notice, ok := noticeFromHits(noticeHits, end)
		if !ok {
			return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "披露日期不明，不能登记为事实证据")
		}
		published, available := conservativeDayTimes(notice)
		if published.After(run.AsOf) || available.After(run.AsOf) {
			return nil, NewError(400, "validation", "FUTURE_EVIDENCE", "晚于 as_of 的材料排除")
		}
		recs = append(recs, financialRecord(inst, end, unit, originals, ms, published, available, retrieved))
	}
	return recs, nil
}

func financialRecord(inst ListedInstrument, end, unit string, originals []string, ms []Metric, published, available, retrieved time.Time) ProviderRecord {
	text := fmt.Sprintf("%s %s 年报已披露指标（%s，合并口径）：%s。来源东方财富。", inst.Name, end, unit, strings.Join(originals, "；"))
	text = NormalizeText(text)
	sourceURL := emSourceURL(inst)
	kind := "eastmoney_hsf10"
	if inst.Market == MarketHK {
		kind = "eastmoney_hkf10"
	}
	return ProviderRecord{
		InstrumentID: inst.ID,
		SourceID:     kind + "_" + inst.Symbol + "_" + end,
		SourceURL:    sourceURL,
		SourceKind:   kind,
		Title:        inst.Name + " " + end + " 年度财务数据",
		Locator:      locatorFor(sourceURL, text),
		Text:         text,
		Metrics:      ms,
		PublishedAt:  published,
		AvailableAt:  available,
		RetrievedAt:  retrieved,
		DataVersion:  DataVersion,
		Mode:         ModeLive,
		Basis:        defaultBasis(strings.Join(originals, ","), unit),
	}
}

func sheetLabel(k statementKind) string {
	switch k {
	case sheetIncome:
		return "利润表"
	case sheetBalance:
		return "资产负债表"
	default:
		return "现金流量表"
	}
}

func noticeFromHits(hits []cninfoAnnouncement, periodEnd string) (time.Time, bool) {
	year := periodEnd
	if len(year) >= 4 {
		year = year[:4]
	}
	for _, hit := range hits {
		title := stripHTMLTags(hit.AnnouncementTitle)
		if strings.Contains(title, year) {
			if day, ok := cninfoTime(hit.AnnouncementTime); ok {
				return day, true
			}
		}
	}
	if len(hits) > 0 {
		return cninfoTime(hits[0].AnnouncementTime)
	}
	return time.Time{}, false
}

func (s *LiveSource) Filings(ctx context.Context, run RunSnapshot, query string, limit int) ([]ProviderRecord, int, error) {
	inst, err := s.resolveInstrument(run.InstrumentID)
	if err != nil {
		return nil, 0, err
	}
	query = strings.TrimSpace(query)
	if query == "" || utf8Len(query) > 300 {
		return nil, 0, NewError(400, "validation", "INVALID_QUERY", "检索词须为 1 至 300 字")
	}
	if strings.Contains(query, "://") || strings.ContainsAny(query, ";\n") {
		return nil, 0, NewError(400, "validation", "INVALID_QUERY", "禁止 URL/SQL/路径参数")
	}
	httpc := NewCountedHTTP(FilingHTTPLimit, ToolTimeout)
	hits, err := searchCninfo(ctx, httpc, inst, query, limit)
	if err != nil {
		return nil, httpc.Count(), err
	}
	retrieved := s.now()
	var recs []ProviderRecord
	reads := 0
	for _, hit := range hits {
		if reads >= 5 {
			break
		}
		if hit.AdjunctURL == "" {
			continue
		}
		fileURL := cninfoFileBase + strings.TrimLeft(hit.AdjunctURL, "/")
		raw, ct, err := httpc.Get(ctx, fileURL)
		reads++
		if err != nil {
			continue
		}
		text := ""
		if strings.Contains(strings.ToLower(ct), "pdf") || bytes.HasPrefix(raw, []byte("%PDF-")) {
			text, err = extractPDFText(raw, 5, 8000)
			if err != nil {
				continue
			}
		} else {
			text = NormalizeText(string(raw))
			if utf8Len(text) > 8000 {
				text = string([]rune(text)[:8000])
			}
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		day, ok := cninfoTime(hit.AnnouncementTime)
		if !ok {
			continue
		}
		published, available := conservativeDayTimes(day)
		if published.After(run.AsOf) || available.After(run.AsOf) {
			continue
		}
		title := stripHTMLTags(hit.AnnouncementTitle)
		recs = append(recs, ProviderRecord{
			InstrumentID: inst.ID,
			SourceID:     "cninfo_" + hit.AnnouncementID,
			SourceURL:    fileURL,
			SourceKind:   "cninfo_filing",
			Title:        title,
			Locator:      locatorFor(fileURL, text),
			Text:         text,
			Metrics:      []Metric{},
			PublishedAt:  published,
			AvailableAt:  available,
			RetrievedAt:  retrieved,
			DataVersion:  DataVersion,
			Mode:         ModeLive,
			Basis: RecordBasis{
				OriginalValue: "", OriginalUnit: "", ScaleFactor: "1", Consolidation: "n/a",
				DatePrecision: "day", AvailabilityBasis: "conservative_day_end",
			},
		})
	}
	if len(recs) == 0 {
		return nil, httpc.Count(), NewError(503, "unavailable", "DATA_UNAVAILABLE", "巨潮命中但正文不足")
	}
	return recs, httpc.Count(), nil
}

func validateFinancialArgs(metrics, periods []string) error {
	if len(metrics) < 1 || len(metrics) > len(MetricDefs) {
		return NewError(400, "validation", "INVALID_METRICS", "metrics 须为目录内 1 至 11 个三表科目")
	}
	if len(periods) < 1 || len(periods) > 2 {
		return NewError(400, "validation", "INVALID_PERIODS", "periods 须为 1 至 2 个年度期末日")
	}
	seen := map[string]struct{}{}
	for _, m := range metrics {
		if _, ok := metricDef(m); !ok {
			return NewError(400, "validation", "UNSUPPORTED_METRIC", "指标不在冻结目录")
		}
		if _, ok := seen[m]; ok {
			return NewError(400, "validation", "INVALID_METRICS", "metrics 不得重复")
		}
		seen[m] = struct{}{}
	}
	for _, p := range periods {
		end := normalizePeriodEnd(p)
		if !strings.HasSuffix(end, "-12-31") {
			return NewError(400, "validation", "PERIOD_NOT_ANNUAL", "仅接受年度期末日")
		}
	}
	return nil
}

func normalizePeriodEnd(period string) string {
	period = strings.TrimSpace(period)
	if len(period) == 4 {
		return period + "-12-31"
	}
	return period
}

func utf8Len(s string) int { return len([]rune(s)) }
