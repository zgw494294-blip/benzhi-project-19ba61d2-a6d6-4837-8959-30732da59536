package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/webui"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

const defaultAddress = "127.0.0.1:19081"

type config struct {
	address          string
	dataDirectory    string
	selfcheck        bool
	selfcheckTimeout time.Duration
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("MuralMortarGate 退出", "error", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("mortar-gate", flag.ContinueOnError)
	address := flags.String("addr", defaultAddress, "HTTP 回环监听地址")
	dataDirectory := flags.String("data-dir", "data", "事件账本数据目录")
	selfcheck := flags.Bool("selfcheck", false, "运行真实 HTTP 业务自检后退出")
	selfcheckTimeout := flags.Duration("selfcheck-timeout", 20*time.Second, "自检总时限")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	explicitAddress := false
	flags.Visit(func(item *flag.Flag) {
		if item.Name == "addr" {
			explicitAddress = true
		}
	})
	resolved := *address
	if !explicitAddress {
		if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
			if _, err := strconv.Atoi(port); err != nil {
				return fmt.Errorf("PORT 必须是端口号: %w", err)
			}
			resolved = net.JoinHostPort("127.0.0.1", port)
		}
	}
	if err := validateAddress(resolved); err != nil {
		return err
	}
	if *selfcheckTimeout <= 0 || *selfcheckTimeout > time.Minute {
		return errors.New("selfcheck-timeout 必须大于 0 且不超过 1 分钟")
	}
	return serve(config{address: resolved, dataDirectory: *dataDirectory, selfcheck: *selfcheck, selfcheckTimeout: *selfcheckTimeout})
}

func serve(cfg config) error {
	dataDirectory := cfg.dataDirectory
	if cfg.selfcheck {
		temporary, err := os.MkdirTemp("", "mural-mortar-selfcheck-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temporary)
		dataDirectory = temporary
	}
	absolute, err := filepath.Abs(dataDirectory)
	if err != nil {
		return err
	}
	store, err := ledger.Open(absolute)
	if err != nil {
		return fmt.Errorf("打开事件账本: %w", err)
	}
	defer store.Close()
	service := workflow.NewService(store)
	var selfcheckEndpoint func() error
	if cfg.selfcheck {
		selfcheckEndpoint = func() error {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.selfcheckTimeout)
			defer cancel()
			_, err := workflow.RunBoundedSelfcheck(ctx, service)
			return err
		}
	}
	handler := webui.NewServer(service, selfcheckEndpoint)
	listener, err := net.Listen("tcp", cfg.address)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.address, err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	serveErrors := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrors <- err
		}
		close(serveErrors)
	}()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("壁画修复灰浆放行台已启动", "address", listener.Addr().String(), "data_dir", absolute, "selfcheck", cfg.selfcheck)
	if cfg.selfcheck {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.selfcheckTimeout)
		defer cancel()
		checkErr := performHTTPSelfcheck(ctx, "http://"+listener.Addr().String())
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer shutdownCancel()
		shutdownErr := server.Shutdown(shutdownContext)
		serverErr := <-serveErrors
		if checkErr != nil {
			return fmt.Errorf("selfcheck 失败: %w", checkErr)
		}
		if shutdownErr != nil {
			return shutdownErr
		}
		if serverErr != nil {
			return serverErr
		}
		logger.Info("selfcheck 完成", "result", "整改复验闭环且放行凭据摘要一致")
		return nil
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case signalValue := <-stop:
		logger.Info("收到停止信号", "signal", signalValue.String())
	case err := <-serveErrors:
		if err != nil {
			return err
		}
		return nil
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return server.Shutdown(shutdownContext)
}

type apiEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func requestJSON(ctx context.Context, client *http.Client, method, url string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("解析 %s 响应: %w", url, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if envelope.Error != nil {
			return fmt.Errorf("%s: %s", envelope.Error.Code, envelope.Error.Message)
		}
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if output != nil {
		return json.Unmarshal(envelope.Data, output)
	}
	return nil
}

func performHTTPSelfcheck(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	var health map[string]string
	if err := requestJSON(ctx, client, http.MethodGet, baseURL+"/healthz", nil, &health); err != nil {
		return err
	}
	var result workflow.ActionResult
	create := workflow.CreateTrialCommand{WriteMeta: workflow.WriteMeta{ExpectedVersion: 0, IdempotencyKey: "http-selfcheck-create-01"}, ID: "HTTP-SELF-CHECK", SiteName: "自检古建壁画", WallSection: "东壁下段", SubstrateCondition: "夯土基底局部酥碱", Owner: "材料研究员", AcceptanceThresholds: domain.Thresholds{MaxColorDifference: 2, MaxShrinkagePct: 1, MinBondStrengthMPa: .3, MaxPowderingGrade: 1}}
	if err := requestJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/trials", create, &result); err != nil {
		return err
	}
	prepared := time.Now().UTC().AddDate(0, 0, -8)
	panel := workflow.RegisterPanelCommand{WriteMeta: workflow.WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "http-selfcheck-panel-001"}, Formula: domain.MortarFormula{ID: "HTTP-F-1", Revision: 1, Components: []domain.FormulaComponent{{Name: "熟石灰", Percentage: 70, BatchRef: "L-01"}, {Name: "细砂", Percentage: 30, BatchRef: "S-01"}}, WaterRatio: .42, MixingMethod: "低速湿拌五分钟", PreparedBy: "材料研究员", PreparedAt: prepared, TemperatureC: 20, HumidityPct: 55}, Panel: domain.TestPanel{ID: "HTTP-P-1", PanelCode: "HTTP-P01", CuringStartedAt: prepared, ScheduledCheckpoints: []int{7}}, Actor: "材料研究员"}
	if err := requestJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/trials/HTTP-SELF-CHECK/panels", panel, &result); err != nil {
		return err
	}
	measurement := workflow.RecordMeasurementCommand{WriteMeta: workflow.WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "http-selfcheck-measure-1"}, PanelID: "HTTP-P-1", Measurement: domain.Measurement{ID: "HTTP-M-1", CheckpointDay: 7, ColorDifference: 2.8, ShrinkagePct: .7, BondStrengthMPa: .4, PowderingGrade: 1, Observation: "色差超限，其余指标稳定", MeasuredBy: "检验员"}}
	if err := requestJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/trials/HTTP-SELF-CHECK/measurements", measurement, &result); err != nil {
		return err
	}
	deviationID := result.Task.Panels[0].Deviations[0].ID
	review := workflow.ReviewDeviationCommand{WriteMeta: workflow.WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "http-selfcheck-review-01"}, PanelID: "HTTP-P-1", DeviationID: deviationID, Conclusion: "remediation_required", Reviewer: "保护责任人", Note: "调整砂源综合色相后复验"}
	if err := requestJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/trials/HTTP-SELF-CHECK/reviews", review, &result); err != nil {
		return err
	}
	remediation := workflow.RemediationCommand{WriteMeta: workflow.WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "http-selfcheck-plan-0001"}, PanelID: "HTTP-P-1", Plan: domain.RemediationPlan{ID: "HTTP-R-1", DeviationIDs: []string{deviationID}, Action: "更换低色度同质细砂并重制复验面", DueDate: time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02"), Responsible: "材料研究员"}, Actor: "保护责任人"}
	if err := requestJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/trials/HTTP-SELF-CHECK/remediations", remediation, &result); err != nil {
		return err
	}
	retest := workflow.RetestCommand{WriteMeta: workflow.WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "http-selfcheck-retest-01"}, PanelID: "HTTP-P-1", PlanID: "HTTP-R-1", Measurement: domain.Measurement{ID: "HTTP-M-R1", ColorDifference: 1.3, ShrinkagePct: .6, BondStrengthMPa: .45, PowderingGrade: 0, Observation: "综合色差与表面状态合格", MeasuredBy: "复验员"}}
	if err := requestJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/trials/HTTP-SELF-CHECK/retests", retest, &result); err != nil {
		return err
	}
	var detail workflow.TrialDetailView
	if err := requestJSON(ctx, client, http.MethodGet, baseURL+"/api/v1/trials/HTTP-SELF-CHECK?approvedBy="+url.QueryEscape("现场施工负责人"), nil, &detail); err != nil {
		return err
	}
	if len(detail.EligibilityMatrix) != 1 || !detail.EligibilityMatrix[0].Eligible {
		return errors.New("冻结预检候选不可用")
	}
	freeze := workflow.FreezeCommand{WriteMeta: workflow.WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "http-selfcheck-freeze-01"}, FormulaID: "HTTP-F-1", PanelID: "HTTP-P-1", ApprovedBy: "现场施工负责人", EligibilityDigest: detail.EligibilityMatrix[0].EligibilityDigest}
	if err := requestJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/trials/HTTP-SELF-CHECK/freeze", freeze, &result); err != nil {
		return err
	}
	release := workflow.ReleaseCommand{WriteMeta: workflow.WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "http-selfcheck-release-1"}, ApprovedBy: "现场施工负责人"}
	if err := requestJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/trials/HTTP-SELF-CHECK/release", release, &result); err != nil {
		return err
	}
	if result.Credential == nil {
		return errors.New("签发响应缺少放行凭据")
	}
	var view workflow.CredentialView
	if err := requestJSON(ctx, client, http.MethodGet, baseURL+"/api/v1/credentials/"+result.Credential.CredentialNo, nil, &view); err != nil {
		return err
	}
	if !view.DigestValid || view.TaskStatus != domain.StatusReleased || len(view.Audit) < 8 {
		return errors.New("凭据摘要、状态或审计轨迹核验失败")
	}
	return nil
}
