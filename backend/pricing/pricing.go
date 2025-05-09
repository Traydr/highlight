package pricing

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/highlight-run/highlight/backend/env"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice"
	mpeTypes "github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice/types"
	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering"
	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v78"

	"github.com/highlight-run/highlight/backend/redis"
	"github.com/highlight-run/highlight/backend/store"

	"github.com/openlyinc/pointy"
	e "github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/sendgrid/sendgrid-go"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/highlight-run/highlight/backend/clickhouse"
	"github.com/highlight-run/highlight/backend/email"
	"github.com/highlight-run/highlight/backend/model"
	backend "github.com/highlight-run/highlight/backend/private-graph/graph/model"
	"github.com/highlight-run/highlight/backend/util"
)

const (
	highlightProductType             string = "highlightProductType"
	highlightProductTier             string = "highlightProductTier"
	highlightProductUnlimitedMembers string = "highlightProductUnlimitedMembers"
	highlightRetentionPeriod         string = "highlightRetentionPeriod"
)

type AWSMPProductCode = string

const AWSMPProductCodeUsageBased AWSMPProductCode = "24dmmonsy3i8lrvjcct8mq07y"

var AWSMPProducts = map[AWSMPProductCode]backend.PlanType{
	AWSMPProductCodeUsageBased: backend.PlanTypeUsageBased,
}

type GraduatedPriceItem struct {
	Rate  float64
	Count int64
}

type ProductPricing struct {
	Included int64
	Items    []GraduatedPriceItem
}

var ProductPrices = map[backend.PlanType]map[model.PricingProductType]ProductPricing{
	backend.PlanTypeGraduated: {
		model.PricingProductTypeSessions: {
			Included: 500,
			Items: []GraduatedPriceItem{{
				Rate:  20. / 1_000,
				Count: 15_000,
			}, {
				Rate:  15. / 1_000,
				Count: 50_000,
			}, {
				Rate:  12. / 1_000,
				Count: 150_000,
			}, {
				Rate:  6.5 / 1_000,
				Count: 500_000,
			}, {
				Rate:  3.5 / 1_000,
				Count: 1_000_000,
			}, {
				Rate: 2.5 / 1_000,
			}},
		},
		model.PricingProductTypeErrors: {
			Included: 1_000,
			Items: []GraduatedPriceItem{{
				Rate:  2. / 1_000,
				Count: 50_000,
			}, {
				Rate:  0.5 / 1_000,
				Count: 100_000,
			}, {
				Rate:  0.25 / 1_000,
				Count: 200_000,
			}, {
				Rate:  0.2 / 1_000,
				Count: 500_000,
			}, {
				Rate:  0.1 / 1_000,
				Count: 5_000_000,
			}, {
				Rate: 0.05 / 1_000,
			}},
		},
		model.PricingProductTypeLogs: {
			Included: 1_000_000,
			Items: []GraduatedPriceItem{{
				Rate:  2.5 / 1_000_000,
				Count: 1_000_000,
			}, {
				Rate:  2. / 1_000_000,
				Count: 10_000_000,
			}, {
				Rate:  1.5 / 1_000_000,
				Count: 100_000_000,
			}, {
				Rate:  1. / 1_000_000,
				Count: 1_000_000_000,
			}, {
				Rate: 0.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeTraces: {
			Included: 25_000_000,
			Items: []GraduatedPriceItem{{
				Rate:  2.5 / 1_000_000,
				Count: 1_000_000,
			}, {
				Rate:  2. / 1_000_000,
				Count: 10_000_000,
			}, {
				Rate:  1.5 / 1_000_000,
				Count: 100_000_000,
			}, {
				Rate:  1. / 1_000_000,
				Count: 1_000_000_000,
			}, {
				Rate: 0.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeMetrics: {
			Included: 1_000,
			Items: []GraduatedPriceItem{{
				Rate:  2.5 / 1_000,
				Count: 1_000,
			}, {
				Rate:  2. / 1_000,
				Count: 10_000,
			}, {
				Rate:  1.5 / 1_000,
				Count: 100_000,
			}, {
				Rate:  1. / 1_000,
				Count: 1_000_000,
			}, {
				Rate: 0.5 / 1_000,
			}},
		},
	},
	backend.PlanTypeUsageBased: {
		model.PricingProductTypeSessions: {
			Included: 500,
			Items: []GraduatedPriceItem{{
				Rate: 20. / 1_000,
			}},
		},
		model.PricingProductTypeErrors: {
			Included: 1_000,
			Items: []GraduatedPriceItem{{
				Rate: 2. / 1_000,
			}},
		},
		model.PricingProductTypeLogs: {
			Included: 1_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeTraces: {
			Included: 1_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeMetrics: {
			Included: 1_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000,
			}},
		},
	},
	backend.PlanTypeLite: {
		model.PricingProductTypeSessions: {
			Included: 2_000,
			Items: []GraduatedPriceItem{{
				Rate: 5. / 1_000,
			}},
		},
		model.PricingProductTypeErrors: {
			Included: 4_000,
			Items: []GraduatedPriceItem{{
				Rate: 0.2 / 1_000,
			}},
		},
		model.PricingProductTypeLogs: {
			Included: 4_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeTraces: {
			Included: 4_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeMetrics: {
			Included: 2_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000,
			}},
		},
	},
	backend.PlanTypeBasic: {
		model.PricingProductTypeSessions: {
			Included: 10_000,
			Items: []GraduatedPriceItem{{
				Rate: 5. / 1_000,
			}},
		},
		model.PricingProductTypeErrors: {
			Included: 20_000,
			Items: []GraduatedPriceItem{{
				Rate: 0.2 / 1_000,
			}},
		},
		model.PricingProductTypeLogs: {
			Included: 20_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeTraces: {
			Included: 20_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeMetrics: {
			Included: 3_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000,
			}},
		},
	},
	backend.PlanTypeStartup: {
		model.PricingProductTypeSessions: {
			Included: 80_000,
			Items: []GraduatedPriceItem{{
				Rate: 5. / 1_000,
			}},
		},
		model.PricingProductTypeErrors: {
			Included: 160_000,
			Items: []GraduatedPriceItem{{
				Rate: 0.2 / 1_000,
			}},
		},
		model.PricingProductTypeLogs: {
			Included: 160_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeTraces: {
			Included: 160_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeMetrics: {
			Included: 6_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000,
			}},
		},
	},
	backend.PlanTypeEnterprise: {
		model.PricingProductTypeSessions: {
			Included: 300_000,
			Items: []GraduatedPriceItem{{
				Rate: 5. / 1_000,
			}},
		},
		model.PricingProductTypeErrors: {
			Included: 600_000,
			Items: []GraduatedPriceItem{{
				Rate: 0.2 / 1_000,
			}},
		},
		model.PricingProductTypeLogs: {
			Included: 600_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeTraces: {
			Included: 600_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeMetrics: {
			Included: 24_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000,
			}},
		},
	},
	backend.PlanTypeFree: {
		model.PricingProductTypeSessions: {
			Included: 500,
			Items: []GraduatedPriceItem{{
				Rate: 5. / 1_000,
			}},
		},
		model.PricingProductTypeErrors: {
			Included: 1_000,
			Items: []GraduatedPriceItem{{
				Rate: 0.2 / 1_000,
			}},
		},
		model.PricingProductTypeLogs: {
			Included: 1_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeTraces: {
			Included: 25_000_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000_000,
			}},
		},
		model.PricingProductTypeMetrics: {
			Included: 1_000,
			Items: []GraduatedPriceItem{{
				Rate: 1.5 / 1_000,
			}},
		},
	},
}

func GetSessions7DayAverage(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, workspace *model.Workspace) (float64, error) {
	var avg float64
	if err := DB.WithContext(ctx).Raw(`
			SELECT COALESCE(AVG(count), 0) as trailingAvg
			FROM daily_session_counts_view
			WHERE project_id in (SELECT id FROM projects WHERE workspace_id=?)
			AND date >= now() - INTERVAL '8 days'
			AND date < now() - INTERVAL '1 day'`, workspace.ID).
		Scan(&avg).Error; err != nil {
		return 0, e.Wrap(err, "error querying for session meter")
	}
	return avg, nil
}

func GetWorkspaceSessionsMeter(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, redisClient *redis.Client, workspace *model.Workspace) (int64, error) {
	meterSpan, _ := util.StartSpanFromContext(ctx, "GetWorkspaceSessionsMeter",
		util.ResourceName("GetWorkspaceSessionsMeter"),
		util.Tag("workspace_id", workspace.ID))
	defer meterSpan.Finish()

	res, err := redis.CachedEval(ctx, redisClient, fmt.Sprintf(`workspace-sessions-meter-%d`, workspace.ID), time.Minute, time.Hour, func() (*int64, error) {
		var meter int64
		if err := DB.WithContext(ctx).Raw(`
		WITH billing_start AS (
			SELECT COALESCE(next_invoice_date - interval '1 month', billing_period_start, date_trunc('month', now(), 'UTC'))
			FROM workspaces
			WHERE id=@workspace_id
		),
		billing_end AS (
			SELECT COALESCE(next_invoice_date, billing_period_end, date_trunc('month', now(), 'UTC') + interval '1 month')
			FROM workspaces
			WHERE id=@workspace_id
		),
		materialized_rows AS (
			SELECT count, date
			FROM daily_session_counts_view
			WHERE project_id in (SELECT id FROM projects WHERE workspace_id=@workspace_id)
			AND date >= (SELECT * FROM billing_start)
			AND date < (SELECT * FROM billing_end)
		),
		start_date as (SELECT COALESCE(MAX(date), (SELECT * from billing_start)) FROM materialized_rows)
		SELECT SUM(count) as currentPeriodSessionCount from (
			SELECT COUNT(*) FROM sessions
			WHERE project_id IN (SELECT id FROM projects WHERE workspace_id=@workspace_id)
			AND created_at >= (SELECT * FROM start_date)
			AND created_at < (SELECT * FROM billing_end)
			AND excluded <> true
			AND within_billing_quota
			AND (active_length >= 1000 OR (active_length is null and length >= 1000))
			AND processed = true
			UNION ALL SELECT COALESCE(SUM(count), 0) FROM materialized_rows
			WHERE date < (SELECT MAX(date) FROM materialized_rows)
		) a`, sql.Named("workspace_id", workspace.ID)).
			Scan(&meter).Error; err != nil {
			return nil, e.Wrap(err, "error querying for session meter")
		}
		return &meter, nil
	})
	return pointy.Int64Value(res, 0), err
}

func GetErrors7DayAverage(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, workspace *model.Workspace) (float64, error) {
	var avg float64
	if err := DB.WithContext(ctx).Raw(`
			SELECT COALESCE(AVG(count), 0) as trailingAvg
			FROM daily_error_counts_view
			WHERE project_id in (SELECT id FROM projects WHERE workspace_id=?)
			AND date >= now() - INTERVAL '8 days'
			AND date < now() - INTERVAL '1 day'`, workspace.ID).
		Scan(&avg).Error; err != nil {
		return 0, e.Wrap(err, "error querying for session meter")
	}
	return avg, nil
}

func GetWorkspaceErrorsMeter(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, redisClient *redis.Client, workspace *model.Workspace) (int64, error) {
	meterSpan, _ := util.StartSpanFromContext(ctx, "GetWorkspaceErrorsMeter",
		util.ResourceName("GetWorkspaceErrorsMeter"),
		util.Tag("workspace_id", workspace.ID))
	defer meterSpan.Finish()

	res, err := redis.CachedEval(ctx, redisClient, fmt.Sprintf(`workspace-errors-meter-%d`, workspace.ID), time.Minute, time.Hour, func() (*int64, error) {
		var meter int64
		if err := DB.WithContext(ctx).Raw(`
		WITH billing_start AS (
			SELECT COALESCE(next_invoice_date - interval '1 month', billing_period_start, date_trunc('month', now(), 'UTC'))
			FROM workspaces
			WHERE id=@workspace_id
		),
		billing_end AS (
			SELECT COALESCE(next_invoice_date, billing_period_end, date_trunc('month', now(), 'UTC') + interval '1 month')
			FROM workspaces
			WHERE id=@workspace_id
		),
		materialized_rows AS (
			SELECT count, date
			FROM daily_error_counts_view
			WHERE project_id in (SELECT id FROM projects WHERE workspace_id=@workspace_id)
			AND date >= (SELECT * FROM billing_start)
			AND date < (SELECT * FROM billing_end)
		),
		start_date as (SELECT COALESCE(MAX(date), (SELECT * from billing_start)) FROM materialized_rows)
		SELECT SUM(count) as currentPeriodErrorCount from (
			SELECT COUNT(*) FROM error_objects
			WHERE project_id IN (SELECT id FROM projects WHERE workspace_id=@workspace_id)
			AND created_at >= (SELECT * FROM start_date)
			AND created_at < (SELECT * FROM billing_end)
			UNION ALL SELECT COALESCE(SUM(count), 0) FROM materialized_rows
			WHERE date < (SELECT MAX(date) FROM materialized_rows)
		) a`, sql.Named("workspace_id", workspace.ID)).
			Scan(&meter).Error; err != nil {
			return nil, e.Wrap(err, "error querying for error meter")
		}
		return &meter, nil
	})
	return pointy.Int64Value(res, 0), err
}

func get7DayAverageImpl(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, workspace *model.Workspace, productType model.PricingProductType) (float64, error) {
	startDate := time.Now().AddDate(0, 0, -8)
	endDate := time.Now().AddDate(0, 0, -1)
	projectIds := lo.Map(workspace.Projects, func(p model.Project, _ int) int {
		return p.ID
	})

	var avgFn func(ctx context.Context, projectIds []int, dateRange backend.DateRangeRequiredInput) (float64, error)
	switch productType {
	case model.PricingProductTypeLogs:
		avgFn = ccClient.ReadLogsDailyAverage
	case model.PricingProductTypeTraces:
		avgFn = ccClient.ReadTracesDailyAverage
	case model.PricingProductTypeMetrics:
		avgFn = ccClient.ReadMetricsDailyAverage
	default:
		return 0, fmt.Errorf("invalid product type %s", productType)
	}

	return avgFn(ctx, projectIds, backend.DateRangeRequiredInput{StartDate: startDate, EndDate: endDate})
}

func getWorkspaceMeterImpl(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, workspace *model.Workspace, productType model.PricingProductType) (int64, error) {
	var startDate time.Time
	if workspace.NextInvoiceDate != nil {
		startDate = workspace.NextInvoiceDate.AddDate(0, -1, 0)
	} else if workspace.BillingPeriodStart != nil {
		startDate = *workspace.BillingPeriodStart
	} else {
		currentYear, currentMonth, _ := time.Now().Date()
		startDate = time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, time.UTC)
	}

	var endDate time.Time
	if workspace.NextInvoiceDate != nil {
		endDate = *workspace.NextInvoiceDate
	} else if workspace.BillingPeriodEnd != nil {
		endDate = *workspace.BillingPeriodEnd
	} else {
		currentYear, currentMonth, _ := time.Now().Date()
		endDate = time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	}

	projectIds := lo.Map(workspace.Projects, func(p model.Project, _ int) int {
		return p.ID
	})

	var sumFn func(ctx context.Context, projectIds []int, dateRange backend.DateRangeRequiredInput) (uint64, error)
	switch productType {
	case model.PricingProductTypeLogs:
		sumFn = ccClient.ReadLogsDailySum
	case model.PricingProductTypeTraces:
		sumFn = ccClient.ReadTracesDailySum
	case model.PricingProductTypeMetrics:
		sumFn = ccClient.ReadMetricsDailySum
	default:
		return 0, fmt.Errorf("invalid product type %s", productType)
	}

	count, err := sumFn(ctx, projectIds, backend.DateRangeRequiredInput{StartDate: startDate, EndDate: endDate})
	if err != nil {
		return 0, err
	}

	return int64(count), nil
}

func GetLogs7DayAverage(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, workspace *model.Workspace) (float64, error) {
	return get7DayAverageImpl(ctx, DB, ccClient, workspace, model.PricingProductTypeLogs)
}

func GetWorkspaceLogsMeter(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, redis *redis.Client, workspace *model.Workspace) (int64, error) {
	return getWorkspaceMeterImpl(ctx, DB, ccClient, workspace, model.PricingProductTypeLogs)
}

func GetTraces7DayAverage(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, workspace *model.Workspace) (float64, error) {
	return get7DayAverageImpl(ctx, DB, ccClient, workspace, model.PricingProductTypeTraces)
}

func GetWorkspaceTracesMeter(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, redis *redis.Client, workspace *model.Workspace) (int64, error) {
	return getWorkspaceMeterImpl(ctx, DB, ccClient, workspace, model.PricingProductTypeTraces)
}

func GetWorkspaceMetricsMeter(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, redis *redis.Client, workspace *model.Workspace) (int64, error) {
	return getWorkspaceMeterImpl(ctx, DB, ccClient, workspace, model.PricingProductTypeMetrics)
}

func GetLimitAmount(limitCostCents *int, productType model.PricingProductType, planType backend.PlanType, retentionPeriod backend.RetentionPeriod) *int64 {
	count := IncludedAmount(planType, productType)
	if planType == backend.PlanTypeFree {
		return pointy.Int64(count)
	}
	if limitCostCents == nil {
		return nil
	}

	retentionMultiplier := RetentionMultiplier(retentionPeriod)
	var cost float64
	for _, item := range ProductPrices[planType][productType].Items {
		quota := int64((float64(*limitCostCents)/100. - cost) / item.Rate / retentionMultiplier)
		if item.Count > 0 && quota > item.Count {
			quota = item.Count
		}
		cost += float64(quota) * item.Rate
		count += quota
		if item.Count == 0 || (item.Count > 0 && quota < item.Count) {
			break
		}
	}

	return pointy.Int64(count)
}

func ProductToBasePriceCents(productType model.PricingProductType, planType backend.PlanType, meter int64) float64 {
	included := IncludedAmount(planType, productType)
	remainder := meter - included
	if remainder <= 0 {
		return ProductPrices[planType][productType].Items[0].Rate * 100.
	}
	var price float64
	for _, item := range ProductPrices[planType][productType].Items {
		if remainder <= 0 {
			break
		}
		itemUsage := remainder
		if item.Count > 0 && itemUsage > item.Count {
			itemUsage = item.Count
		}
		price += float64(itemUsage) * item.Rate
		remainder -= itemUsage
	}
	return price / float64(meter-included) * 100.
}

func RetentionMultiplier(retentionPeriod backend.RetentionPeriod) float64 {
	switch retentionPeriod {
	case backend.RetentionPeriodSevenDays:
		return 1
	case backend.RetentionPeriodThirtyDays:
		return 1
	case backend.RetentionPeriodThreeMonths:
		return 1
	case backend.RetentionPeriodSixMonths:
		return 1.5
	case backend.RetentionPeriodTwelveMonths:
		return 2
	case backend.RetentionPeriodTwoYears:
		return 2.5
	case backend.RetentionPeriodThreeYears:
		return 3
	default:
		return 1
	}
}

func TypeToMemberLimit(planType backend.PlanType, unlimitedMembers bool) *int64 {
	if unlimitedMembers {
		return nil
	}
	switch planType {
	case backend.PlanTypeBasic:
		return pointy.Int64(2)
	case backend.PlanTypeStartup:
		return pointy.Int64(8)
	case backend.PlanTypeEnterprise:
		return pointy.Int64(15)
	default:
		return pointy.Int64(2)
	}
}

func IncludedAmount(planType backend.PlanType, productType model.PricingProductType) int64 {
	return ProductPrices[planType][productType].Included
}

func FromPriceID(priceID string) backend.PlanType {
	switch priceID {
	case env.Config.PricingBasicPriceID:
		return backend.PlanTypeBasic
	case env.Config.PricingStartupPriceID:
		return backend.PlanTypeStartup
	case env.Config.PricingEnterprisePriceID:
		return backend.PlanTypeEnterprise
	}
	return backend.PlanTypeFree
}

// MustUpgradeForClearbit shows when tier is insufficient for Clearbit.
func MustUpgradeForClearbit(tier string) bool {
	pt := backend.PlanType(tier)
	return pt != backend.PlanTypeStartup && pt != backend.PlanTypeEnterprise
}

// Returns a Stripe lookup key which maps to a single Stripe Price
func GetBaseLookupKey(productTier backend.PlanType, interval model.PricingSubscriptionInterval, unlimitedMembers bool, retentionPeriod backend.RetentionPeriod) (result string) {
	switch productTier {
	case backend.PlanTypeUsageBased:
		return fmt.Sprintf("%s|%s", model.PricingProductTypeBase, backend.PlanTypeUsageBased)
	case backend.PlanTypeGraduated:
		return fmt.Sprintf("%s|%s", model.PricingProductTypeBase, backend.PlanTypeGraduated)
	default:
		result = fmt.Sprintf("%s|%s|%s", model.PricingProductTypeBase, string(productTier), string(interval))
		if unlimitedMembers {
			result += "|UNLIMITED_MEMBERS"
		}
		if retentionPeriod != backend.RetentionPeriodThreeMonths {
			result += "|" + string(retentionPeriod)
		}
	}
	return
}

func GetOverageKey(productType model.PricingProductType, retentionPeriod backend.RetentionPeriod, planType backend.PlanType) string {
	result := string(productType)
	if retentionPeriod != backend.RetentionPeriodThreeMonths {
		result += "|" + string(retentionPeriod)
	}

	if planType == backend.PlanTypeGraduated {
		result += "|" + backend.PlanTypeGraduated.String()
	} else if planType == backend.PlanTypeUsageBased && productType == model.PricingProductTypeSessions {
		result += "|" + backend.PlanTypeUsageBased.String()
	}
	return result
}

// Returns the Highlight model.PricingProductType, Tier, and Interval for the Stripe Price
func GetProductMetadata(price *stripe.Price) (*model.PricingProductType, *backend.PlanType, bool, model.PricingSubscriptionInterval, backend.RetentionPeriod) {
	interval := model.PricingSubscriptionIntervalMonthly
	if price.Recurring != nil && price.Recurring.Interval == stripe.PriceRecurringIntervalYear {
		interval = model.PricingSubscriptionIntervalAnnual
	}

	retentionPeriod := backend.RetentionPeriodSixMonths

	// If the price id corresponds to a tier using the old conversion,
	// return it for backward compatibility
	oldTier := FromPriceID(price.ID)
	if oldTier != backend.PlanTypeFree {
		base := model.PricingProductTypeBase
		return &base, &oldTier, false, interval, retentionPeriod
	}

	var productTypePtr *model.PricingProductType
	var tierPtr *backend.PlanType

	if typeStr, ok := price.Product.Metadata[highlightProductType]; ok {
		productType := model.PricingProductType(typeStr)
		productTypePtr = &productType
	}

	if tierStr, ok := price.Product.Metadata[highlightProductTier]; ok {
		tier := backend.PlanType(tierStr)
		tierPtr = &tier
	}

	unlimitedMembers := false
	if unlimitedMembersStr, ok := price.Product.Metadata[highlightProductUnlimitedMembers]; ok {
		if unlimitedMembersStr == "true" {
			unlimitedMembers = true
		}
	}

	if retentionStr, ok := price.Metadata[highlightRetentionPeriod]; ok {
		retentionPeriod = backend.RetentionPeriod(retentionStr)
	}

	return productTypePtr, tierPtr, unlimitedMembers, interval, retentionPeriod
}

// Products are too nested in the Subscription model to be added through the API
// This method calls the Stripe ListProducts API and replaces each product id in the
// subscriptions with the full product data.
func FillProducts(pricingClient *Client, subscriptions []*stripe.Subscription) {
	productListParams := &stripe.ProductListParams{}
	for _, subscription := range subscriptions {
		for _, subscriptionItem := range subscription.Items.Data {
			productListParams.IDs = append(productListParams.IDs, &subscriptionItem.Price.Product.ID)
		}
	}

	productsById := map[string]*stripe.Product{}
	if len(productListParams.IDs) > 0 {
		// Loop over each product in the subscription
		products := pricingClient.Products.List(productListParams).ProductList().Data
		for _, product := range products {
			productsById[product.ID] = product
		}
	}

	for _, subscription := range subscriptions {
		for _, subscriptionItem := range subscription.Items.Data {
			productId := subscriptionItem.Price.Product.ID
			if product, ok := productsById[productId]; ok {
				subscriptionItem.Price.Product = product
			}
		}
	}
}

// Returns the Stripe Prices for the associated tier and interval
func GetStripePrices(pricingClient *Client, workspace *model.Workspace, productTier backend.PlanType, interval model.PricingSubscriptionInterval, unlimitedMembers bool, sessionsRetention *backend.RetentionPeriod, errorsRetention *backend.RetentionPeriod) (map[model.PricingProductType]*stripe.Price, error) {
	// Default to the `RetentionPeriodThreeMonths` prices for customers grandfathered into 6 month retention
	sessionsRetentionPeriod := backend.RetentionPeriodThreeMonths
	if sessionsRetention != nil {
		sessionsRetentionPeriod = *sessionsRetention
	}
	errorsRetentionPeriod := backend.RetentionPeriodThreeMonths
	if errorsRetention != nil {
		errorsRetentionPeriod = *errorsRetention
	}
	baseLookupKey := GetBaseLookupKey(productTier, interval, unlimitedMembers, sessionsRetentionPeriod)

	membersLookupKey := string(model.PricingProductTypeMembers)
	sessionsLookupKey := GetOverageKey(model.PricingProductTypeSessions, sessionsRetentionPeriod, productTier)
	errorsLookupKey := GetOverageKey(model.PricingProductTypeErrors, errorsRetentionPeriod, productTier)
	// logs and traces are only available with three month retention
	logsLookupKey := GetOverageKey(model.PricingProductTypeLogs, backend.RetentionPeriodThreeMonths, productTier)
	tracesLookupKey := GetOverageKey(model.PricingProductTypeTraces, backend.RetentionPeriodThreeMonths, productTier)
	metricsLookupKey := GetOverageKey(model.PricingProductTypeMetrics, backend.RetentionPeriodThreeMonths, productTier)

	priceListParams := stripe.PriceListParams{}
	priceListParams.LookupKeys = []*string{&baseLookupKey, &sessionsLookupKey, &membersLookupKey, &errorsLookupKey, &logsLookupKey, &tracesLookupKey, &metricsLookupKey}
	prices := pricingClient.Prices.List(&priceListParams).PriceList().Data

	priceMap := map[model.PricingProductType]*stripe.Price{}
	for _, price := range prices {
		switch price.LookupKey {
		case baseLookupKey:
			priceMap[model.PricingProductTypeBase] = price
		case sessionsLookupKey:
			priceMap[model.PricingProductTypeSessions] = price
		case membersLookupKey:
			priceMap[model.PricingProductTypeMembers] = price
		case errorsLookupKey:
			priceMap[model.PricingProductTypeErrors] = price
		case logsLookupKey:
			priceMap[model.PricingProductTypeLogs] = price
		case tracesLookupKey:
			priceMap[model.PricingProductTypeTraces] = price
		case metricsLookupKey:
			priceMap[model.PricingProductTypeMetrics] = price
		}
	}

	// fill values for custom price overrides
	for product, priceID := range map[model.PricingProductType]*string{
		model.PricingProductTypeSessions: workspace.StripeSessionOveragePriceID,
		model.PricingProductTypeErrors:   workspace.StripeErrorOveragePriceID,
		model.PricingProductTypeLogs:     workspace.StripeLogOveragePriceID,
		model.PricingProductTypeTraces:   workspace.StripeTracesOveragePriceID,
		model.PricingProductTypeMetrics:  workspace.StripeMetricsOveragePriceID,
	} {
		if priceID != nil {
			price, err := pricingClient.Prices.Get(*priceID, &stripe.PriceParams{})
			if err != nil {
				return nil, err
			}
			priceMap[product] = price
		}
	}

	expected := len(priceListParams.LookupKeys)
	actual := len(priceMap)
	if actual != expected {
		searchedKeys := lo.Map(priceListParams.LookupKeys, func(key *string, _ int) string {
			return *key
		})
		foundProducts := lo.Keys(priceMap)
		return nil, e.Errorf("expected %d prices, received %d; searched %#v, found %#v", expected, actual, searchedKeys, foundProducts)
	}

	return priceMap, nil
}

type Worker struct {
	db            *gorm.DB
	redis         *redis.Client
	store         *store.Store
	ccClient      *clickhouse.Client
	awsmpClient   *marketplacemetering.Client
	mailClient    *sendgrid.Client
	pricingClient *Client
}

func NewWorker(db *gorm.DB, redis *redis.Client, store *store.Store, ccClient *clickhouse.Client, pricingClient *Client, awsmpClient *marketplacemetering.Client, mailClient *sendgrid.Client) *Worker {
	return &Worker{
		db:            db,
		redis:         redis,
		store:         store,
		ccClient:      ccClient,
		pricingClient: pricingClient,
		awsmpClient:   awsmpClient,
		mailClient:    mailClient,
	}
}

func (w *Worker) ReportStripeUsageForWorkspace(ctx context.Context, workspaceID int) error {
	return w.reportStripeUsage(ctx, workspaceID)
}

type WorkspaceOverages = map[model.PricingProductType]int64
type AWSCustomerUsage struct {
	Customer *model.AWSMarketplaceCustomer
	Usage    WorkspaceOverages
}
type AWSCustomerUsages = map[int]AWSCustomerUsage

func (w *Worker) ReportAWSMPUsages(ctx context.Context, usages AWSCustomerUsages) {
	var now = time.Now()
	var usageRecords []types.UsageRecord
	for workspaceID, usage := range usages {
		for product, overage := range usage.Usage {
			if int64(int32(overage)) != overage {
				log.WithContext(ctx).WithField("workspaceID", workspaceID).WithField("product", product).WithField("overage", overage).Error("BILLING_ERROR aws mp overage overflowed")
				continue
			}
			usageRecords = append(usageRecords, types.UsageRecord{
				CustomerIdentifier: usage.Customer.CustomerIdentifier,
				Timestamp:          &now,
				Dimension:          pointy.String(strings.ToLower(string(product))),
				Quantity:           pointy.Int32(int32(overage)),
			})
		}
	}
	for _, chunk := range lo.Chunk(usageRecords, 25) {
		if _, err := w.awsmpClient.BatchMeterUsage(ctx, &marketplacemetering.BatchMeterUsageInput{
			ProductCode:  pointy.String(AWSMPProductCodeUsageBased),
			UsageRecords: chunk,
		}); err != nil {
			log.WithContext(ctx).WithError(err).Error("BILLING_ERROR failed to report aws mp usages")
		}
		log.WithContext(ctx).WithField("chunk", chunk).Infof("reported aws mp usage for %d records", len(chunk))
	}
}

type overageConfig struct {
	MaxCostCents          func(*model.Workspace) *int
	Meter                 func(ctx context.Context, DB *gorm.DB, ccClient *clickhouse.Client, redisClient *redis.Client, workspace *model.Workspace) (int64, error)
	RetentionPeriod       func(*model.Workspace) backend.RetentionPeriod
	Included              func(*model.Workspace) int64
	OverageEmail          email.EmailType
	OverageEmailThreshold int64
}

var ProductTypeToQuotaConfig = map[model.PricingProductType]overageConfig{
	model.PricingProductTypeSessions: {
		func(w *model.Workspace) *int { return w.SessionsMaxCents },
		GetWorkspaceSessionsMeter,
		func(w *model.Workspace) backend.RetentionPeriod {
			if w.RetentionPeriod == nil {
				return backend.RetentionPeriodThreeMonths
			}
			return *w.RetentionPeriod
		},
		func(w *model.Workspace) int64 {
			limit := IncludedAmount(backend.PlanType(w.PlanTier), model.PricingProductTypeSessions)
			if w.MonthlySessionLimit != nil {
				limit = int64(*w.MonthlySessionLimit)
			}
			return limit
		},
		email.BillingSessionOverage,
		1000,
	},
	model.PricingProductTypeErrors: {
		func(w *model.Workspace) *int { return w.ErrorsMaxCents },
		GetWorkspaceErrorsMeter,
		func(w *model.Workspace) backend.RetentionPeriod {
			if w.ErrorsRetentionPeriod == nil {
				return backend.RetentionPeriodThreeMonths
			}
			return *w.ErrorsRetentionPeriod
		},
		func(w *model.Workspace) int64 {
			limit := IncludedAmount(backend.PlanType(w.PlanTier), model.PricingProductTypeErrors)
			if w.MonthlyErrorsLimit != nil {
				limit = int64(*w.MonthlyErrorsLimit)
			}
			return limit
		},
		email.BillingErrorsOverage,
		1000,
	},
	model.PricingProductTypeLogs: {
		func(w *model.Workspace) *int { return w.LogsMaxCents },
		GetWorkspaceLogsMeter,
		func(w *model.Workspace) backend.RetentionPeriod {
			if w.LogsRetentionPeriod == nil {
				return backend.RetentionPeriodThirtyDays
			}
			return *w.LogsRetentionPeriod
		},
		func(w *model.Workspace) int64 {
			limit := IncludedAmount(backend.PlanType(w.PlanTier), model.PricingProductTypeLogs)
			if w.MonthlyLogsLimit != nil {
				limit = int64(*w.MonthlyLogsLimit)
			}
			return limit
		},
		email.BillingLogsOverage,
		1_000_000,
	},
	model.PricingProductTypeTraces: {
		func(w *model.Workspace) *int { return w.TracesMaxCents },
		GetWorkspaceTracesMeter,
		func(w *model.Workspace) backend.RetentionPeriod {
			if w.TracesRetentionPeriod == nil {
				return backend.RetentionPeriodThirtyDays
			}
			return *w.TracesRetentionPeriod
		},
		func(w *model.Workspace) int64 {
			limit := IncludedAmount(backend.PlanType(w.PlanTier), model.PricingProductTypeTraces)
			if w.MonthlyTracesLimit != nil {
				limit = int64(*w.MonthlyTracesLimit)
			}
			return limit
		},
		email.BillingTracesOverage,
		1_000_000,
	},
	model.PricingProductTypeMetrics: {
		func(w *model.Workspace) *int { return w.MetricsMaxCents },
		GetWorkspaceMetricsMeter,
		func(w *model.Workspace) backend.RetentionPeriod {
			if w.MetricsRetentionPeriod == nil {
				return backend.RetentionPeriodThirtyDays
			}
			return *w.MetricsRetentionPeriod
		},
		func(w *model.Workspace) int64 {
			limit := IncludedAmount(backend.PlanType(w.PlanTier), model.PricingProductTypeMetrics)
			if w.MonthlyMetricsLimit != nil {
				limit = int64(*w.MonthlyMetricsLimit)
			}
			return limit
		},
		email.BillingMetricsOverage,
		1_000,
	},
}

func (w *Worker) CalculateOverages(ctx context.Context, workspaceID int) (WorkspaceOverages, error) {
	var workspace *model.Workspace
	var err error
	if workspace, err = w.store.GetWorkspace(ctx, workspaceID); err != nil {
		return nil, e.Wrap(err, "error querying workspace")
	}

	var usage = make(WorkspaceOverages)
	// Update members overage
	membersMeter, err := w.store.GetWorkspaceAdminCount(ctx, workspaceID)
	if err != nil {
		return nil, e.Wrap(err, "failed to query workspace admins meter")
	}
	membersLimit := TypeToMemberLimit(backend.PlanType(workspace.PlanTier), workspace.UnlimitedMembers)
	if membersLimit != nil && workspace.MonthlyMembersLimit != nil {
		membersLimit = pointy.Int64(int64(*workspace.MonthlyMembersLimit))
	}
	usage[model.PricingProductTypeMembers] = calculateOverage(workspace, membersLimit, membersMeter)

	for product, cfg := range ProductTypeToQuotaConfig {
		meter, err := cfg.Meter(ctx, w.db, w.ccClient, w.redis, workspace)
		if err != nil {
			return nil, e.Wrapf(err, "BILLING_ERROR error getting %s meter", product)
		}
		included := cfg.Included(workspace)
		usage[product] = calculateOverage(workspace, &included, meter)
		if meter > included+cfg.OverageEmailThreshold {
			if err := model.SendBillingNotifications(ctx, w.db, w.mailClient, cfg.OverageEmail, workspace, nil); err != nil {
				log.WithContext(ctx).Error(e.Wrap(err, "failed to send billing notifications"))
			}
		}
	}
	return usage, nil
}

func (w *Worker) reportStripeUsage(ctx context.Context, workspaceID int) error {
	var workspace *model.Workspace
	var err error
	if workspace, err = w.store.GetWorkspace(ctx, workspaceID); err != nil {
		return e.Wrap(err, "error querying workspace")
	}

	// If the trial end date is recent (within the past 7 days) or it hasn't ended yet
	// The 7 day check is to avoid sending emails to customers whose trials ended long ago
	if workspace.TrialEndDate != nil && workspace.TrialEndDate.After(time.Now().AddDate(0, 0, -7)) {
		if workspace.TrialEndDate.Before(time.Now()) {
			// If the trial has ended, send an email
			if err := model.SendBillingNotifications(ctx, w.db, w.mailClient, email.BillingHighlightTrialEnded, workspace, nil); err != nil {
				log.WithContext(ctx).Error(e.Wrap(err, "failed to send billing notifications"))
			}
		} else if workspace.TrialEndDate.Before(time.Now().AddDate(0, 0, 7)) {
			// If the trial is ending within 7 days, send an email
			if err := model.SendBillingNotifications(ctx, w.db, w.mailClient, email.BillingHighlightTrial7Days, workspace, nil); err != nil {
				log.WithContext(ctx).Error(e.Wrap(err, "failed to send billing notifications"))
			}
		}
	}

	// Don't report usage for free plans
	if backend.PlanType(workspace.PlanTier) == backend.PlanTypeFree {
		return nil
	}

	if workspace.BillingPeriodStart == nil ||
		workspace.BillingPeriodEnd == nil ||
		time.Now().Before(*workspace.BillingPeriodStart) ||
		!time.Now().Before(*workspace.BillingPeriodEnd) {
		return e.New("workspace billing period is not valid")
	}

	customerParams := &stripe.CustomerParams{}
	customerParams.AddExpand("subscriptions")
	customerParams.AddExpand("subscriptions.data.discount")
	customerParams.AddExpand("subscriptions.data.discount.coupon")
	c, err := w.pricingClient.Customers.Get(*workspace.StripeCustomerID, customerParams)
	if err != nil {
		return e.Wrap(err, "couldn't retrieve stripe customer data")
	}

	if len(c.Subscriptions.Data) > 1 {
		return e.New("BILLING_ERROR cannot report usage - customer has multiple subscriptions")
	} else if len(c.Subscriptions.Data) == 0 {
		return e.New("BILLING_ERROR cannot report usage - customer has no subscriptions")
	}

	subscriptions := c.Subscriptions.Data
	FillProducts(w.pricingClient, subscriptions)

	subscription := subscriptions[0]

	if len(lo.Filter(subscription.Items.Data, func(item *stripe.SubscriptionItem, _ int) bool {
		return item.Price.Recurring.UsageType != stripe.PriceRecurringUsageTypeMetered
	})) != 1 {
		return e.New("BILLING_ERROR cannot report usage - subscription has multiple products")
	}

	baseProductItem, ok := lo.Find(subscription.Items.Data, func(item *stripe.SubscriptionItem) bool {
		_, ok := item.Price.Product.Metadata[highlightProductType]
		return ok
	})
	if !ok {
		return e.New("BILLING_ERROR cannot report usage - cannot find base product")
	}

	_, productTier, _, interval, _ := GetProductMetadata(baseProductItem.Price)
	if productTier == nil {
		return e.New("BILLING_ERROR cannot report usage - product has no tier")
	}

	// If the subscription has a 100% coupon with an expiration
	if subscription.Discount != nil &&
		subscription.Discount.Coupon != nil &&
		subscription.Discount.Coupon.PercentOff == 100 &&
		subscription.Discount.End != 0 {
		subscriptionEnd := time.Unix(subscription.Discount.End, 0)
		if subscriptionEnd.Before(time.Now().AddDate(0, 0, 3)) {
			// If the Stripe trial is ending within 3 days, send an email
			if err := model.SendBillingNotifications(ctx, w.db, w.mailClient, email.BillingStripeTrial3Days, workspace, nil); err != nil {
				log.WithContext(ctx).Error(e.Wrap(err, "BILLING_ERROR failed to send billing notifications"))
			}
		} else if subscriptionEnd.Before(time.Now().AddDate(0, 0, 7)) {
			// If the Stripe trial is ending within 7 days, send an email
			if err := model.SendBillingNotifications(ctx, w.db, w.mailClient, email.BillingStripeTrial7Days, workspace, nil); err != nil {
				log.WithContext(ctx).Error(e.Wrap(err, "BILLING_ERROR failed to send billing notifications"))
			}
		}
	}

	// For non-monthly subscriptions, set PendingInvoiceItemInterval to 'month' if not set
	// so that overage is reported via monthly invoice items.
	if interval != model.PricingSubscriptionIntervalMonthly && (subscription.PendingInvoiceItemInterval == nil || subscription.PendingInvoiceItemInterval.Interval != stripe.SubscriptionPendingInvoiceItemIntervalIntervalMonth) {
		log.WithContext(ctx).WithField("workspaceID", workspaceID).Info("configuring monthly invoices for non-monthly subscription")
		updated, err := w.pricingClient.Subscriptions.Update(subscription.ID, &stripe.SubscriptionParams{
			PendingInvoiceItemInterval: &stripe.SubscriptionPendingInvoiceItemIntervalParams{
				Interval: stripe.String(string(stripe.SubscriptionPendingInvoiceItemIntervalIntervalMonth)),
			},
		})
		if err != nil {
			return e.Wrap(err, "BILLING_ERROR failed to update PendingInvoiceItemInterval")
		}

		if updated.NextPendingInvoiceItemInvoice != 0 {
			timestamp := time.Unix(updated.NextPendingInvoiceItemInvoice, 0)
			if err := w.db.WithContext(ctx).Model(workspace).Where("id = ?", workspaceID).
				Updates(&model.Workspace{
					NextInvoiceDate: &timestamp,
				}).Error; err != nil {
				return e.Wrapf(err, "BILLING_ERROR error updating workspace NextInvoiceDate")
			}
		}
	}

	prices, err := GetStripePrices(w.pricingClient, workspace, *productTier, interval, workspace.UnlimitedMembers, workspace.RetentionPeriod, workspace.ErrorsRetentionPeriod)
	if err != nil {
		return e.Wrap(err, "BILLING_ERROR cannot report usage - failed to get Stripe prices")
	}

	invoiceParams := &stripe.InvoiceUpcomingParams{
		Customer:     &c.ID,
		Subscription: &subscription.ID,
	}

	invoice, err := w.pricingClient.Invoices.Upcoming(invoiceParams)
	// Cancelled subscriptions have no upcoming invoice - we can skip these since we won't
	// be charging any overage for their next billing period.
	if err != nil {
		log.WithContext(ctx).WithField("workspaceID", workspaceID).WithError(err).Warn("workspace has no invoice upcoming, will not report overage")
		return nil
	}

	invoiceLinesParams := &stripe.InvoiceUpcomingLinesParams{
		Customer:     &c.ID,
		Subscription: &subscription.ID,
	}
	invoiceLinesParams.AddExpand("data.price.product")

	i := w.pricingClient.Invoices.UpcomingLines(invoiceLinesParams)
	if err = i.Err(); err != nil {
		return e.Wrap(err, "BILLING_ERROR cannot report usage - failed to retrieve invoice lines for customer "+c.ID)
	}
	var lineItems []*stripe.InvoiceLineItem
	for i.Next() {
		lineItems = append(lineItems, i.InvoiceLineItem())
	}

	invoiceLines := map[model.PricingProductType]*stripe.InvoiceLineItem{}
	// GroupBy to remove will extra line items
	// duplicates are present because graduated pricing (one invoice item)
	// has more than one invoice line item for each bucket's price.
	// ie. price of `First 4999` and `Next 19999` are two different line items for the same subscription item.
	grouped := lo.GroupBy(lineItems, func(item *stripe.InvoiceLineItem) string {
		if item.SubscriptionItem != nil {
			return item.SubscriptionItem.ID
		}
		return ""
	})
	for subscriptionItem, group := range grouped {
		if len(group) == 0 {
			return e.Wrapf(err, "BILLING_ERROR empty group, failed to group invoice lines for %s", subscriptionItem)
		}
		// if the subscriptionItem is not set, these are non-graduated line items that we want to delete
		// if set, we only want to keep the first line item
		if subscriptionItem != "" {
			group = []*stripe.InvoiceLineItem{group[0]}
		}
		for _, line := range group {
			productType, _, _, _, _ := GetProductMetadata(line.Price)
			if productType != nil {
				// if the line is from an old price, warn so we can check and manually delete it
				if line.Price.ID != prices[*productType].ID {
					log.WithContext(ctx).Warnf("STRIPE_INTEGRATION_WARN mismatched invoice line item %s existing %s expected %s for customer %s", line.ID, line.Price.ID, prices[*productType].ID, c.ID)
				} else {
					invoiceLines[*productType] = line
				}
			}
		}
	}
	log.WithContext(ctx).WithField("invoiceLinesLen", len(invoiceLines)).Infof("STRIPE_INTEGRATION_INFO found invoice lines %d %+v", len(invoiceLines), invoiceLines)

	billingIssue, err := w.GetBillingIssue(ctx, workspace, c, subscription, invoice)
	if err != nil {
		log.WithContext(ctx).WithError(err).WithField("customer", c.ID).Error("BILLING_ERROR failed to get billing issue status")
	} else {
		w.ProcessBillingIssue(ctx, workspace, billingIssue)
	}

	overages, err := w.CalculateOverages(ctx, workspaceID)
	if err != nil {
		return e.Wrap(err, "BILLING_ERROR error calculating workspace overages")
	}

	for _, productType := range []model.PricingProductType{
		model.PricingProductTypeMembers,
		model.PricingProductTypeSessions,
		model.PricingProductTypeErrors,
		model.PricingProductTypeLogs,
		model.PricingProductTypeTraces,
		model.PricingProductTypeMetrics,
	} {
		if err := w.AddOrUpdateOverageItem(prices[productType], invoiceLines[productType], c, subscription, overages[productType]); err != nil {
			return e.Wrapf(err, "BILLING_ERROR error updating overage item for product %s", productType)
		}
	}

	return nil
}

type PaymentIssueType = string

const PaymentIssueTypeSubscriptionDue PaymentIssueType = "subscription_due"
const PaymentIssueTypeInvoiceUncollectible PaymentIssueType = "invoice_uncollectible"
const PaymentIssueTypeInvoiceOpenAttempted PaymentIssueType = "invoice_open_attempted"
const PaymentIssueTypeNoPaymentMethod PaymentIssueType = "no_payment_method"
const PaymentIssueTypeCardCheckFail PaymentIssueType = "payment_method_check_failed"

func (w *Worker) GetBillingIssue(ctx context.Context, workspace *model.Workspace, customer *stripe.Customer, subscription *stripe.Subscription, invoice *stripe.Invoice) (PaymentIssueType, error) {
	settings, err := w.store.GetAllWorkspaceSettings(ctx, workspace.ID)
	if err != nil {
		return "", err
	}
	if !settings.CanShowBillingIssueBanner {
		return "", err
	}

	if invalid := map[stripe.SubscriptionStatus]bool{
		stripe.SubscriptionStatusIncomplete: true,
		stripe.SubscriptionStatusPastDue:    true,
		stripe.SubscriptionStatusUnpaid:     true,
	}[subscription.Status]; invalid {
		log.WithContext(ctx).WithField("customer", customer.ID).WithField("subscription_status", subscription.Status).Info("stripe unpaid invoice detected", invoice.ID)
		return PaymentIssueTypeSubscriptionDue, nil
	}

	if invoice != nil && invoice.Status == stripe.InvoiceStatusUncollectible {
		log.WithContext(ctx).WithField("customer", customer.ID).Info("stripe uncollectible invoice detected", invoice.ID)
		return PaymentIssueTypeInvoiceUncollectible, nil
	}

	if invoice != nil && invoice.Status == stripe.InvoiceStatusOpen {
		if invoice.AttemptCount > 0 {
			log.WithContext(ctx).WithField("customer", customer.ID).Info("stripe invoice found with failed attempts", invoice.ID)
			return PaymentIssueTypeInvoiceOpenAttempted, nil
		}
	}

	// check for valid CC to make sure customer is valid
	i := w.pricingClient.PaymentMethods.List(&stripe.PaymentMethodListParams{Customer: pointy.String(customer.ID)})
	if err := i.Err(); err != nil {
		return "", err
	}

	if len(i.PaymentMethodList().Data) == 0 {
		log.WithContext(ctx).WithField("customer", customer.ID).Info("no payment methods found")
		return PaymentIssueTypeNoPaymentMethod, nil
	}

	var failures, paymentMethods int
	for _, paymentMethod := range i.PaymentMethodList().Data {
		paymentMethods += 1
		if paymentMethod.Card != nil && paymentMethod.Card.Checks != nil {
			if paymentMethod.Card.Checks.CVCCheck == stripe.PaymentMethodCardChecksCVCCheckFail && paymentMethod.Card.Checks.AddressPostalCodeCheck == stripe.PaymentMethodCardChecksAddressPostalCodeCheckFail {
				log.WithContext(ctx).
					WithField("customer", customer.ID).
					WithField("checks", *paymentMethod.Card.Checks).
					Info("stripe cvc+zip check failed")
				failures += 1
			}
		}
	}
	if failures >= paymentMethods {
		return PaymentIssueTypeCardCheckFail, nil
	}

	return "", nil
}

const BillingWarningPeriod = 7 * 24 * time.Hour

func (w *Worker) ProcessBillingIssue(ctx context.Context, workspace *model.Workspace, status PaymentIssueType) {
	if status == "" {
		if err := w.redis.SetCustomerBillingWarning(ctx, ptr.ToString(workspace.StripeCustomerID), time.Time{}); err != nil {
			log.WithContext(ctx).WithError(err).Error("BILLING_ERROR failed to clear customer billing warning status")
		}

		if err := w.redis.SetCustomerBillingInvalid(ctx, ptr.ToString(workspace.StripeCustomerID), false); err != nil {
			log.WithContext(ctx).WithError(err).Error("BILLING_ERROR failed to clear customer invalid billing status")
		}

		return
	}

	warningSent, err := w.redis.GetCustomerBillingWarning(ctx, ptr.ToString(workspace.StripeCustomerID))
	if err != nil {
		log.WithContext(ctx).WithError(err).Error("BILLING_ERROR failed to get customer invalid billing warning status")
		return
	}

	if warningSent.IsZero() {
		if err = model.SendBillingNotifications(ctx, w.db, w.mailClient, email.BillingInvalidPayment, workspace, ptr.String(status)); err != nil {
			log.WithContext(ctx).WithError(err).Error("BILLING_ERROR failed to send customer invalid billing warning notification")
			return
		}
		warningSent = time.Now()
	}
	// keep setting the warning time to save that this customer has had a warning before
	if err := w.redis.SetCustomerBillingWarning(ctx, ptr.ToString(workspace.StripeCustomerID), warningSent); err != nil {
		log.WithContext(ctx).WithError(err).Error("BILLING_ERROR failed to set customer billing warning status")
	}

	if time.Since(warningSent) > BillingWarningPeriod {
		if err := w.redis.SetCustomerBillingInvalid(ctx, ptr.ToString(workspace.StripeCustomerID), true); err != nil {
			log.WithContext(ctx).WithError(err).Error("BILLING_ERROR failed to set customer invalid billing status")
		}
	}
}

func calculateOverage(workspace *model.Workspace, limit *int64, meter int64) int64 {
	// Calculate overage if the workspace allows it
	overage := int64(0)
	if limit != nil &&
		backend.PlanType(workspace.PlanTier) != backend.PlanTypeFree &&
		workspace.AllowMeterOverage && meter > *limit {
		overage = meter - *limit
	}
	return overage
}

func (w *Worker) AddOrUpdateOverageItem(newPrice *stripe.Price, invoiceLine *stripe.InvoiceLineItem, customer *stripe.Customer, subscription *stripe.Subscription, overage int64) error {
	// if the price is a metered recurring subscription, use subscription items and usage records
	if newPrice.Recurring != nil {
		var subscriptionItemID string
		// if the subscription item doesn't create for this price, create it
		if invoiceLine == nil || invoiceLine.SubscriptionItem.ID == "" {
			params := &stripe.SubscriptionItemParams{
				Subscription: &subscription.ID,
				Price:        &newPrice.ID,
			}
			params.SetIdempotencyKey(subscription.ID + ":" + newPrice.ID + ":item:" + uuid.New().String())
			subscriptionItem, err := w.pricingClient.SubscriptionItems.New(params)
			if err != nil {
				return e.Wrapf(err, "BILLING_ERROR failed to add invoice item for usage record item; invoiceLine=%+v, priceID=%s, subscriptionID=%s", invoiceLine, newPrice.ID, subscription.ID)
			}
			subscriptionItemID = subscriptionItem.ID
		} else {
			subscriptionItemID = invoiceLine.SubscriptionItem.ID
		}
		// set the usage for this product, replacing existing values
		params := &stripe.UsageRecordParams{
			SubscriptionItem: stripe.String(subscriptionItemID),
			Action:           stripe.String("set"),
			Quantity:         stripe.Int64(overage),
		}
		if _, err := w.pricingClient.UsageRecords.New(params); err != nil {
			return e.Wrap(err, "BILLING_ERROR failed to add usage record item")
		}
	} else {
		if invoiceLine != nil {
			if _, err := w.pricingClient.InvoiceItems.Update(invoiceLine.InvoiceItem.ID, &stripe.InvoiceItemParams{
				Price:    &newPrice.ID,
				Quantity: stripe.Int64(overage),
			}); err != nil {
				return e.Wrap(err, "BILLING_ERROR failed to update invoice item")
			}
		} else {
			params := &stripe.InvoiceItemParams{
				Customer:     &customer.ID,
				Subscription: &subscription.ID,
				Price:        &newPrice.ID,
				Quantity:     stripe.Int64(overage),
			}
			params.SetIdempotencyKey(customer.ID + ":" + subscription.ID + ":" + newPrice.ID + ":" + uuid.New().String())
			if _, err := w.pricingClient.InvoiceItems.New(params); err != nil {
				return e.Wrap(err, "BILLING_ERROR failed to add invoice item")
			}
		}
	}

	return nil
}

func (w *Worker) ReportAllUsage(ctx context.Context) {
	// Get all workspace IDs
	var workspaces []*model.Workspace
	if err := w.db.WithContext(ctx).
		Model(&model.Workspace{}).
		Joins("AWSMarketplaceCustomer").
		Where("billing_period_start is not null").
		Where("billing_period_end is not null").
		Find(&workspaces).Error; err != nil {
		log.WithContext(ctx).Error("failed to query workspaces")
		return
	}

	awsWorkspaceUsages := AWSCustomerUsages{}
	for _, workspace := range workspaces {
		if workspace.AWSMarketplaceCustomer != nil {
			usage, err := w.CalculateOverages(ctx, workspace.ID)
			if err != nil {
				log.WithContext(ctx).
					WithField("workspaceID", workspace.ID).
					Error(e.Wrapf(err, "error calculating aws overages for workspace %d", workspace.ID))
			} else {
				awsWorkspaceUsages[workspace.ID] = AWSCustomerUsage{workspace.AWSMarketplaceCustomer, usage}
				log.WithContext(ctx).
					WithField("workspaceID", workspace.ID).
					WithField("usage", awsWorkspaceUsages[workspace.ID].Usage).
					Info("reporting aws mp overages")
			}
		} else if err := w.reportStripeUsage(ctx, workspace.ID); err != nil {
			log.WithContext(ctx).
				WithField("workspaceID", workspace.ID).
				Error(e.Wrapf(err, "error reporting stripe usage for workspace %d", workspace.ID))
		}
	}
	w.ReportAWSMPUsages(ctx, awsWorkspaceUsages)
}

func GetEntitlements(ctx context.Context, customer *marketplacemetering.ResolveCustomerOutput) ([]mpeTypes.Entitlement, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return nil, err
	}

	var entitlements []mpeTypes.Entitlement
	var page *string
	mpe := marketplaceentitlementservice.NewFromConfig(cfg)
	for {
		ent, err := mpe.GetEntitlements(ctx, &marketplaceentitlementservice.GetEntitlementsInput{
			ProductCode: customer.ProductCode,
			Filter: map[string][]string{
				"CUSTOMER_IDENTIFIER": {pointy.StringValue(customer.CustomerIdentifier, "")},
			},
			MaxResults: pointy.Int32(25),
			NextToken:  page,
		})
		if err != nil {
			return nil, err
		}
		log.WithContext(ctx).
			WithField("customer", pointy.StringValue(customer.CustomerIdentifier, "")).
			WithField("entitlements", ent.Entitlements).
			Info("made entitlement request for customer")

		if len(ent.Entitlements) == 0 || ent.NextToken == nil {
			break
		}
		entitlements = append(entitlements, ent.Entitlements...)
		page = ent.NextToken
	}

	for _, ent := range entitlements {
		log.WithContext(ctx).
			WithField("customer", pointy.StringValue(customer.CustomerIdentifier, "")).
			WithField("entitlement_dimension", pointy.StringValue(ent.Dimension, "")).
			WithField("entitlement_value", ent.Value).
			Info("found entitlement for customer")
	}

	return entitlements, nil
}
