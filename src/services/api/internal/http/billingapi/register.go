package billingapi

import (
	nethttp "net/http"

	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/entitlement"
	"arkloop/services/shared/database"

)

type Deps struct {
	AuthService         *auth.Service
	OrgMembershipRepo   *data.OrgMembershipRepository
	PlansRepo           *data.PlanRepository
	EntitlementsRepo    *data.EntitlementsRepository
	APIKeysRepo         *data.APIKeysRepository
	SubscriptionsRepo   *data.SubscriptionRepository
	EntitlementService  *entitlement.Service
	UsageRepo           *data.UsageRepository
	CreditsRepo         *data.CreditsRepository
	InviteCodesRepo     *data.InviteCodeRepository
	ReferralsRepo       *data.ReferralRepository
	RedemptionCodesRepo *data.RedemptionCodesRepository
	AuditWriter         *audit.Writer
	DB                database.DB
}

func RegisterRoutes(mux *nethttp.ServeMux, deps Deps) {
	mux.HandleFunc("/v1/plans", plansEntry(deps.AuthService, deps.OrgMembershipRepo, deps.PlansRepo, deps.EntitlementsRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/plans/", planEntry(deps.AuthService, deps.OrgMembershipRepo, deps.PlansRepo, deps.EntitlementsRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/subscriptions", subscriptionsEntry(deps.AuthService, deps.OrgMembershipRepo, deps.SubscriptionsRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/subscriptions/", subscriptionEntry(deps.AuthService, deps.OrgMembershipRepo, deps.SubscriptionsRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/entitlement-overrides", entitlementOverridesEntry(deps.AuthService, deps.OrgMembershipRepo, deps.EntitlementsRepo, deps.EntitlementService, deps.APIKeysRepo, deps.AuditWriter))
	mux.HandleFunc("/v1/entitlement-overrides/", entitlementOverrideEntry(deps.AuthService, deps.OrgMembershipRepo, deps.EntitlementsRepo, deps.EntitlementService, deps.APIKeysRepo, deps.AuditWriter))
	mux.HandleFunc("/v1/orgs/{id}/usage", orgUsageEntry(deps.AuthService, deps.OrgMembershipRepo, deps.UsageRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/orgs/{id}/usage/daily", orgDailyUsage(deps.AuthService, deps.OrgMembershipRepo, deps.UsageRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/orgs/{id}/usage/by-model", orgUsageByModel(deps.AuthService, deps.OrgMembershipRepo, deps.UsageRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/admin/usage/daily", adminGlobalDailyUsage(deps.AuthService, deps.OrgMembershipRepo, deps.UsageRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/admin/usage/summary", adminGlobalUsageSummary(deps.AuthService, deps.OrgMembershipRepo, deps.UsageRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/admin/usage/by-model", adminGlobalUsageByModel(deps.AuthService, deps.OrgMembershipRepo, deps.UsageRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/admin/invite-codes", adminInviteCodesEntry(deps.AuthService, deps.OrgMembershipRepo, deps.InviteCodesRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/admin/invite-codes/", adminInviteCodeEntry(deps.AuthService, deps.OrgMembershipRepo, deps.InviteCodesRepo, deps.APIKeysRepo, deps.AuditWriter))
	mux.HandleFunc("/v1/admin/referrals/tree", adminReferralTree(deps.AuthService, deps.OrgMembershipRepo, deps.ReferralsRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/admin/referrals", adminReferralsEntry(deps.AuthService, deps.OrgMembershipRepo, deps.ReferralsRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/admin/credits/adjust", adminCreditsAdjust(deps.AuthService, deps.OrgMembershipRepo, deps.CreditsRepo, deps.APIKeysRepo, deps.AuditWriter))
	mux.HandleFunc("/v1/admin/credits/bulk-adjust", adminCreditsBulkAdjust(deps.AuthService, deps.OrgMembershipRepo, deps.CreditsRepo, deps.APIKeysRepo, deps.AuditWriter))
	mux.HandleFunc("/v1/admin/credits/reset-all", adminCreditsResetAll(deps.AuthService, deps.OrgMembershipRepo, deps.CreditsRepo, deps.APIKeysRepo, deps.AuditWriter))
	mux.HandleFunc("/v1/admin/credits", adminCreditsEntry(deps.AuthService, deps.OrgMembershipRepo, deps.CreditsRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/admin/redemption-codes/batch", adminRedemptionCodesBatch(deps.AuthService, deps.OrgMembershipRepo, deps.RedemptionCodesRepo, deps.APIKeysRepo, deps.AuditWriter, deps.DB))
	mux.HandleFunc("/v1/admin/redemption-codes/", adminRedemptionCodeEntry(deps.AuthService, deps.OrgMembershipRepo, deps.RedemptionCodesRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/admin/redemption-codes", adminRedemptionCodesEntry(deps.AuthService, deps.OrgMembershipRepo, deps.RedemptionCodesRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/me/usage", meUsage(deps.AuthService, deps.OrgMembershipRepo, deps.UsageRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/me/usage/daily", meDailyUsage(deps.AuthService, deps.OrgMembershipRepo, deps.UsageRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/me/usage/by-model", meUsageByModel(deps.AuthService, deps.OrgMembershipRepo, deps.UsageRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/me/invite-code/reset", meInviteCodeReset(deps.AuthService, deps.OrgMembershipRepo, deps.InviteCodesRepo, deps.EntitlementService, deps.APIKeysRepo, deps.AuditWriter))
	mux.HandleFunc("/v1/me/invite-code", meInviteCode(deps.AuthService, deps.OrgMembershipRepo, deps.InviteCodesRepo, deps.EntitlementService, deps.APIKeysRepo, deps.AuditWriter))
	mux.HandleFunc("/v1/me/credits", meCredits(deps.AuthService, deps.OrgMembershipRepo, deps.CreditsRepo, deps.APIKeysRepo))
	mux.HandleFunc("/v1/me/redeem", meRedeem(deps.AuthService, deps.OrgMembershipRepo, deps.RedemptionCodesRepo, deps.CreditsRepo, deps.APIKeysRepo, deps.AuditWriter, deps.DB))
}
