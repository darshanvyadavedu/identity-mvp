package services

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"regexp"
	"strings"
	"time"

	"user-authentication/app/models"
	"user-authentication/app/repositories"
	"user-authentication/config"

	"gorm.io/gorm"
)

// ── Param / Result types ──────────────────────────────────────────────────────

type StartProfessionParams struct {
	UserID           string
	ProfessionType   string
	ProfessionTitle  string
	EmployerName     string
	ConsentStoreData bool
}

type StartProfessionResult struct {
	VerificationID  string `json:"verificationId"`
	Status          string `json:"status"`
	ProfessionType  string `json:"professionType"`
	ConfidenceScore int    `json:"confidenceScore"`
	ConfidenceLevel string `json:"confidenceLevel"`
}

type SubmitWorkEmailParams struct {
	VerificationID string
	UserID         string
	WorkEmail      string
}

type SubmitWorkEmailResult struct {
	EvidenceID string `json:"evidenceId"`
	Message    string `json:"message"`
	// DevOTP is non-empty only when SMTP is not configured (development mode).
	DevOTP string `json:"dev_otp,omitempty"`
}

type ConfirmEmailParams struct {
	VerificationID string
	UserID         string
	EvidenceID     string
	OTP            string
}

type ConfirmEmailResult struct {
	Verified        bool   `json:"verified"`
	ConfidenceScore int    `json:"confidenceScore"`
	ConfidenceLevel string `json:"confidenceLevel"`
}

type SubmitDocumentParams struct {
	VerificationID string
	UserID         string
	DocBytes       []byte
	DocumentType   string
	FileName       string
}

type SubmitDocumentResult struct {
	EvidenceID      string `json:"evidenceId"`
	ConfidenceScore int    `json:"confidenceScore"`
	ConfidenceLevel string `json:"confidenceLevel"`
}

type SubmitPortfolioParams struct {
	VerificationID string
	UserID         string
	URL            string
	Platform       string
}

type SubmitPortfolioResult struct {
	EvidenceID      string `json:"evidenceId"`
	ConfidenceScore int    `json:"confidenceScore"`
	ConfidenceLevel string `json:"confidenceLevel"`
}

type GetProfessionStatusParams struct {
	VerificationID string
	UserID         string
}

type EvidenceSummary struct {
	EvidenceID        string         `json:"evidenceId"`
	Method            string         `json:"method"`
	Status            string         `json:"status"`
	ScoreContribution int            `json:"scoreContribution"`
	Metadata          map[string]any `json:"metadata"`
}

type GetProfessionStatusResult struct {
	VerificationID  string            `json:"verificationId"`
	Status          string            `json:"status"`
	ProfessionType  string            `json:"professionType"`
	ConfidenceScore int               `json:"confidenceScore"`
	ConfidenceLevel string            `json:"confidenceLevel"`
	Evidence        []EvidenceSummary `json:"evidence"`
}

type LinkedInAuthURLParams struct {
	VerificationID string
	UserID         string
}

type LinkedInAuthURLResult struct {
	AuthURL string `json:"authUrl"`
	State   string `json:"state"`
}

type LinkedInCallbackParams struct {
	VerificationID string
	UserID         string
	AuthCode       string
	State          string
}

type LinkedInCallbackResult struct {
	EvidenceID      string `json:"evidenceId"`
	DisplayName     string `json:"displayName"`
	Headline        string `json:"headline"`
	ConfidenceScore int    `json:"confidenceScore"`
	ConfidenceLevel string `json:"confidenceLevel"`
}

// ── Interface ─────────────────────────────────────────────────────────────────

type ProfessionServiceInterface interface {
	StartVerification(ctx context.Context, db *gorm.DB, params *StartProfessionParams) (*StartProfessionResult, ServiceErrorInterface)
	SubmitWorkEmail(ctx context.Context, db *gorm.DB, params *SubmitWorkEmailParams) (*SubmitWorkEmailResult, ServiceErrorInterface)
	ConfirmEmail(ctx context.Context, db *gorm.DB, params *ConfirmEmailParams) (*ConfirmEmailResult, ServiceErrorInterface)
	SubmitDocument(ctx context.Context, db *gorm.DB, params *SubmitDocumentParams) (*SubmitDocumentResult, ServiceErrorInterface)
	SubmitPortfolio(ctx context.Context, db *gorm.DB, params *SubmitPortfolioParams) (*SubmitPortfolioResult, ServiceErrorInterface)
	GetStatus(ctx context.Context, db *gorm.DB, params *GetProfessionStatusParams) (*GetProfessionStatusResult, ServiceErrorInterface)
	GetLinkedInAuthURL(ctx context.Context, db *gorm.DB, params *LinkedInAuthURLParams) (*LinkedInAuthURLResult, ServiceErrorInterface)
	HandleLinkedInCallback(ctx context.Context, db *gorm.DB, params *LinkedInCallbackParams) (*LinkedInCallbackResult, ServiceErrorInterface)
}

// ── Struct & constructor ──────────────────────────────────────────────────────

type professionService struct {
	professionRepo repositories.ProfessionRepoInterface
	auditRepo      repositories.AuditRepoInterface
}

type ProfessionServiceOption func(*professionService)

func NewProfessionService(opts ...ProfessionServiceOption) ProfessionServiceInterface {
	svc := &professionService{
		professionRepo: repositories.NewProfessionRepo(),
		auditRepo:      repositories.NewAuditRepo(),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func ConfigureProfessionRepo(r repositories.ProfessionRepoInterface) ProfessionServiceOption {
	return func(s *professionService) { s.professionRepo = r }
}

func ConfigureProfessionAuditRepo(r repositories.AuditRepoInterface) ProfessionServiceOption {
	return func(s *professionService) { s.auditRepo = r }
}

// ── StartVerification ─────────────────────────────────────────────────────────

func (svc *professionService) StartVerification(_ context.Context, db *gorm.DB, params *StartProfessionParams) (*StartProfessionResult, ServiceErrorInterface) {
	if !isValidProfessionType(params.ProfessionType) {
		return nil, ErrBadRequest("profession_type must be one of: corporate, freelance, regulated")
	}

	// Respect privacy: store profession details only with explicit consent.
	title, employer := "", ""
	if params.ConsentStoreData {
		title = params.ProfessionTitle
		employer = params.EmployerName
	}

	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	v, err := svc.professionRepo.Create(db, &models.ProfessionVerification{
		UserID:           params.UserID,
		ProfessionType:   models.ProfessionType(params.ProfessionType),
		ProfessionTitle:  title,
		EmployerName:     employer,
		Status:           models.ProfessionStatusInProgress,
		ConfidenceScore:  0,
		ConfidenceLevel:  models.ConfidenceLevelNone,
		ConsentStoreData: params.ConsentStoreData,
		ExpiresAt:        &expiresAt,
	})
	if err != nil {
		return nil, ErrInternalServer("create profession verification: " + err.Error())
	}

	details, _ := json.Marshal(map[string]any{
		"profession_type": params.ProfessionType,
		"consent":         params.ConsentStoreData,
	})
	_ = svc.auditRepo.Create(db, &models.AuditLog{
		UserID:    params.UserID,
		Action:    "profession_verification_started",
		SessionID: nullSessionID,
		Details:   details,
	})

	return &StartProfessionResult{
		VerificationID:  v.VerificationID,
		Status:          string(v.Status),
		ProfessionType:  string(v.ProfessionType),
		ConfidenceScore: v.ConfidenceScore,
		ConfidenceLevel: string(v.ConfidenceLevel),
	}, nil
}

// ── SubmitWorkEmail ───────────────────────────────────────────────────────────

// freeEmailDomains lists providers whose email addresses do not indicate professional use.
var freeEmailDomains = map[string]bool{
	"gmail.com": true, "yahoo.com": true, "hotmail.com": true,
	"outlook.com": true, "live.com": true, "icloud.com": true,
	"protonmail.com": true, "proton.me": true, "aol.com": true,
	"mail.com": true, "ymail.com": true, "yahoo.co.in": true,
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func (svc *professionService) SubmitWorkEmail(_ context.Context, db *gorm.DB, params *SubmitWorkEmailParams) (*SubmitWorkEmailResult, ServiceErrorInterface) {
	if !emailRegex.MatchString(params.WorkEmail) {
		return nil, ErrBadRequest("invalid email address")
	}

	v, err := svc.professionRepo.GetByIDAndUser(db, params.VerificationID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("verification not found")
	}
	if v.Status == models.ProfessionStatusVerified {
		return nil, ErrBadRequest("verification is already complete")
	}

	otp, genErr := generateOTP()
	if genErr != nil {
		return nil, ErrInternalServer("generate OTP: " + genErr.Error())
	}

	cfg := config.Get()
	otpHash := computeHMAC(otp, cfg.HMACSecret)
	expires := time.Now().Add(10 * time.Minute)

	domain := extractEmailDomain(params.WorkEmail)
	isProfessional := !freeEmailDomains[strings.ToLower(domain)]

	metadata, _ := json.Marshal(map[string]any{
		"email_domain":    domain,
		"is_professional": isProfessional,
	})

	evidence, err := svc.professionRepo.UpsertEvidence(db, &models.ProfessionEvidence{
		VerificationID:  params.VerificationID,
		UserID:          params.UserID,
		Method:          models.MethodWorkEmail,
		Status:          models.EvidenceStatusPending,
		ScoreContribution: 0,
		Metadata:        metadata,
		EmailOTPHash:    otpHash,
		EmailOTPExpires: &expires,
	})
	if err != nil {
		return nil, ErrInternalServer("save evidence: " + err.Error())
	}

	var devOTP string
	if cfg.SMTPHost == "" {
		devOTP = otp
	} else {
		if sendErr := sendOTPEmail(params.WorkEmail, otp, cfg); sendErr != nil {
			return nil, ErrInternalServer("send OTP email: " + sendErr.Error())
		}
	}

	return &SubmitWorkEmailResult{
		EvidenceID: evidence.EvidenceID,
		Message:    "OTP sent to your work email. Please confirm within 10 minutes.",
		DevOTP:     devOTP,
	}, nil
}

// ── ConfirmEmail ──────────────────────────────────────────────────────────────

func (svc *professionService) ConfirmEmail(_ context.Context, db *gorm.DB, params *ConfirmEmailParams) (*ConfirmEmailResult, ServiceErrorInterface) {
	v, err := svc.professionRepo.GetByIDAndUser(db, params.VerificationID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("verification not found")
	}

	evidence, err := svc.professionRepo.GetEvidenceByID(db, params.EvidenceID, params.VerificationID)
	if err != nil {
		return nil, ErrNotFound("evidence not found")
	}
	if evidence.Method != models.MethodWorkEmail {
		return nil, ErrBadRequest("evidence is not a work email submission")
	}
	if evidence.Status == models.EvidenceStatusVerified {
		return nil, ErrBadRequest("email already verified")
	}
	if evidence.EmailOTPExpires == nil || time.Now().After(*evidence.EmailOTPExpires) {
		return nil, ErrBadRequest("OTP has expired; please resubmit your work email")
	}

	cfg := config.Get()
	if computeHMAC(params.OTP, cfg.HMACSecret) != evidence.EmailOTPHash {
		return nil, ErrBadRequest("invalid OTP")
	}

	// Award points: professional domain earns more than a generic work email.
	var meta map[string]any
	_ = json.Unmarshal(evidence.Metadata, &meta)
	isProfessional, _ := meta["is_professional"].(bool)
	score := 25
	if isProfessional {
		score = 35
	}

	if err := svc.professionRepo.UpdateEvidenceStatus(db, evidence.EvidenceID, string(models.EvidenceStatusVerified), score); err != nil {
		return nil, ErrInternalServer("update evidence: " + err.Error())
	}

	newScore, newLevel := svc.recalculateScore(db, params.VerificationID)
	_ = svc.professionRepo.UpdateScoreAndStatus(db, params.VerificationID, newScore, newLevel, nextStatus(v.Status, newScore))

	details, _ := json.Marshal(map[string]any{"score": newScore, "level": newLevel})
	_ = svc.auditRepo.Create(db, &models.AuditLog{
		UserID:    params.UserID,
		Action:    "profession_email_verified",
		SessionID: nullSessionID,
		Details:   details,
	})

	return &ConfirmEmailResult{Verified: true, ConfidenceScore: newScore, ConfidenceLevel: newLevel}, nil
}

// ── SubmitDocument ────────────────────────────────────────────────────────────

func (svc *professionService) SubmitDocument(_ context.Context, db *gorm.DB, params *SubmitDocumentParams) (*SubmitDocumentResult, ServiceErrorInterface) {
	if len(params.DocBytes) == 0 {
		return nil, ErrBadRequest("document file is required")
	}

	v, err := svc.professionRepo.GetByIDAndUser(db, params.VerificationID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("verification not found")
	}
	if v.Status == models.ProfessionStatusVerified {
		return nil, ErrBadRequest("verification is already complete")
	}

	// Regulated professionals (doctors, lawyers) receive higher confidence for
	// documents because their license numbers can be cross-checked against
	// official registries in future pipeline enhancements.
	score := 35
	if v.ProfessionType == models.ProfessionTypeRegulated {
		score = 45
	}

	metadata, _ := json.Marshal(map[string]any{
		"document_type": params.DocumentType,
		"file_name":     params.FileName,
		"file_size_kb":  len(params.DocBytes) / 1024,
	})

	// Document bytes are intentionally NOT persisted; only metadata is stored.
	evidence, err := svc.professionRepo.UpsertEvidence(db, &models.ProfessionEvidence{
		VerificationID:    params.VerificationID,
		UserID:            params.UserID,
		Method:            models.MethodDocumentUpload,
		Status:            models.EvidenceStatusVerified,
		ScoreContribution: score,
		Metadata:          metadata,
	})
	if err != nil {
		return nil, ErrInternalServer("save evidence: " + err.Error())
	}

	newScore, newLevel := svc.recalculateScore(db, params.VerificationID)
	_ = svc.professionRepo.UpdateScoreAndStatus(db, params.VerificationID, newScore, newLevel, nextStatus(v.Status, newScore))

	return &SubmitDocumentResult{
		EvidenceID:      evidence.EvidenceID,
		ConfidenceScore: newScore,
		ConfidenceLevel: newLevel,
	}, nil
}

// ── SubmitPortfolio ───────────────────────────────────────────────────────────

var urlRegex = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)

func (svc *professionService) SubmitPortfolio(_ context.Context, db *gorm.DB, params *SubmitPortfolioParams) (*SubmitPortfolioResult, ServiceErrorInterface) {
	if !urlRegex.MatchString(params.URL) {
		return nil, ErrBadRequest("invalid URL; must start with http:// or https://")
	}

	v, err := svc.professionRepo.GetByIDAndUser(db, params.VerificationID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("verification not found")
	}
	if v.Status == models.ProfessionStatusVerified {
		return nil, ErrBadRequest("verification is already complete")
	}

	platform := params.Platform
	if platform == "" {
		platform = detectPlatform(params.URL)
	}

	metadata, _ := json.Marshal(map[string]any{
		"url":      params.URL,
		"platform": platform,
	})

	evidence, err := svc.professionRepo.UpsertEvidence(db, &models.ProfessionEvidence{
		VerificationID:    params.VerificationID,
		UserID:            params.UserID,
		Method:            models.MethodPortfolioSocial,
		Status:            models.EvidenceStatusVerified,
		ScoreContribution: 20,
		Metadata:          metadata,
	})
	if err != nil {
		return nil, ErrInternalServer("save evidence: " + err.Error())
	}

	newScore, newLevel := svc.recalculateScore(db, params.VerificationID)
	_ = svc.professionRepo.UpdateScoreAndStatus(db, params.VerificationID, newScore, newLevel, nextStatus(v.Status, newScore))

	return &SubmitPortfolioResult{
		EvidenceID:      evidence.EvidenceID,
		ConfidenceScore: newScore,
		ConfidenceLevel: newLevel,
	}, nil
}

// ── GetStatus ─────────────────────────────────────────────────────────────────

func (svc *professionService) GetStatus(_ context.Context, db *gorm.DB, params *GetProfessionStatusParams) (*GetProfessionStatusResult, ServiceErrorInterface) {
	v, err := svc.professionRepo.GetByIDAndUser(db, params.VerificationID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("verification not found")
	}

	allEvidence, err := svc.professionRepo.ListEvidence(db, params.VerificationID)
	if err != nil {
		return nil, ErrInternalServer("list evidence: " + err.Error())
	}

	summaries := make([]EvidenceSummary, 0, len(allEvidence))
	for _, e := range allEvidence {
		var meta map[string]any
		_ = json.Unmarshal(e.Metadata, &meta)
		summaries = append(summaries, EvidenceSummary{
			EvidenceID:        e.EvidenceID,
			Method:            string(e.Method),
			Status:            string(e.Status),
			ScoreContribution: e.ScoreContribution,
			Metadata:          meta,
		})
	}

	return &GetProfessionStatusResult{
		VerificationID:  v.VerificationID,
		Status:          string(v.Status),
		ProfessionType:  string(v.ProfessionType),
		ConfidenceScore: v.ConfidenceScore,
		ConfidenceLevel: string(v.ConfidenceLevel),
		Evidence:        summaries,
	}, nil
}

// ── LinkedIn OAuth ────────────────────────────────────────────────────────────

func (svc *professionService) GetLinkedInAuthURL(_ context.Context, db *gorm.DB, params *LinkedInAuthURLParams) (*LinkedInAuthURLResult, ServiceErrorInterface) {
	cfg := config.Get()
	if cfg.LinkedInClientID == "" {
		return nil, ErrBadRequest("LinkedIn OAuth is not configured on this server (set LINKEDIN_CLIENT_ID)")
	}
	if _, err := svc.professionRepo.GetByIDAndUser(db, params.VerificationID, params.UserID); err != nil {
		return nil, ErrNotFound("verification not found")
	}

	// State encodes verificationID+userID so we can route the callback correctly.
	state := params.VerificationID + ":" + params.UserID
	authURL := fmt.Sprintf(
		"https://www.linkedin.com/oauth/v2/authorization?response_type=code&client_id=%s&redirect_uri=%s&scope=r_liteprofile&state=%s",
		cfg.LinkedInClientID,
		url.QueryEscape(cfg.LinkedInRedirectURI),
		url.QueryEscape(state),
	)

	return &LinkedInAuthURLResult{AuthURL: authURL, State: state}, nil
}

func (svc *professionService) HandleLinkedInCallback(ctx context.Context, db *gorm.DB, params *LinkedInCallbackParams) (*LinkedInCallbackResult, ServiceErrorInterface) {
	cfg := config.Get()
	if cfg.LinkedInClientID == "" {
		return nil, ErrBadRequest("LinkedIn OAuth is not configured on this server")
	}

	v, err := svc.professionRepo.GetByIDAndUser(db, params.VerificationID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("verification not found")
	}

	token, liErr := exchangeLinkedInCode(ctx, params.AuthCode, cfg)
	if liErr != nil {
		return nil, ErrBadGateway("LinkedIn token exchange: " + liErr.Error())
	}

	profile, liErr := fetchLinkedInProfile(ctx, token)
	if liErr != nil {
		return nil, ErrBadGateway("LinkedIn profile fetch: " + liErr.Error())
	}

	metadata, _ := json.Marshal(map[string]any{
		"linkedin_id":  profile.ID,
		"display_name": profile.DisplayName,
		"headline":     profile.Headline,
	})

	evidence, err := svc.professionRepo.UpsertEvidence(db, &models.ProfessionEvidence{
		VerificationID:    params.VerificationID,
		UserID:            params.UserID,
		Method:            models.MethodLinkedInOAuth,
		Status:            models.EvidenceStatusVerified,
		ScoreContribution: 40,
		Metadata:          metadata,
	})
	if err != nil {
		return nil, ErrInternalServer("save evidence: " + err.Error())
	}

	newScore, newLevel := svc.recalculateScore(db, params.VerificationID)
	_ = svc.professionRepo.UpdateScoreAndStatus(db, params.VerificationID, newScore, newLevel, nextStatus(v.Status, newScore))

	return &LinkedInCallbackResult{
		EvidenceID:      evidence.EvidenceID,
		DisplayName:     profile.DisplayName,
		Headline:        profile.Headline,
		ConfidenceScore: newScore,
		ConfidenceLevel: newLevel,
	}, nil
}

// ── Confidence scoring ────────────────────────────────────────────────────────

// Scoring table:
//   work_email (professional domain): 35 pts
//   work_email (free domain):         25 pts
//   linkedin_oauth:                   40 pts
//   document_upload (regulated):      45 pts
//   document_upload (other):          35 pts
//   portfolio_social:                 20 pts
//   multi-method bonus (≥2 verified): +10 pts
//   cap:                              100 pts
//
// Confidence levels:
//   0      → none
//   1–35   → low
//   36–65  → medium
//   66–100 → high

func (svc *professionService) recalculateScore(db *gorm.DB, verificationID string) (int, string) {
	allEvidence, err := svc.professionRepo.ListEvidence(db, verificationID)
	if err != nil {
		return 0, string(models.ConfidenceLevelNone)
	}

	total, verifiedCount := 0, 0
	for _, e := range allEvidence {
		if e.Status == models.EvidenceStatusVerified {
			total += e.ScoreContribution
			verifiedCount++
		}
	}

	if verifiedCount >= 2 {
		total += 10
	}
	if total > 100 {
		total = 100
	}

	return total, string(levelFromScore(total))
}

func levelFromScore(score int) models.ConfidenceLevel {
	switch {
	case score == 0:
		return models.ConfidenceLevelNone
	case score <= 35:
		return models.ConfidenceLevelLow
	case score <= 65:
		return models.ConfidenceLevelMedium
	default:
		return models.ConfidenceLevelHigh
	}
}

// nextStatus promotes status to "verified" once medium/high confidence is reached.
// Terminal states (failed, expired) are never changed.
func nextStatus(current models.ProfessionStatus, score int) string {
	if current == models.ProfessionStatusFailed || current == models.ProfessionStatusExpired {
		return string(current)
	}
	if current == models.ProfessionStatusVerified || score >= 36 {
		return string(models.ProfessionStatusVerified)
	}
	return string(models.ProfessionStatusInProgress)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// nullSessionID is used for audit log entries that have no associated identity
// session (e.g. profession verification actions). The audit insert silently fails
// if the DB column rejects it; this is acceptable because audit is best-effort.
const nullSessionID = "00000000-0000-0000-0000-000000000000"

func isValidProfessionType(t string) bool {
	switch models.ProfessionType(t) {
	case models.ProfessionTypeCorporate, models.ProfessionTypeFreelance, models.ProfessionTypeRegulated:
		return true
	}
	return false
}

func extractEmailDomain(email string) string {
	if idx := strings.LastIndex(email, "@"); idx >= 0 {
		return strings.ToLower(email[idx+1:])
	}
	return ""
}

func detectPlatform(rawURL string) string {
	lower := strings.ToLower(rawURL)
	platforms := map[string]string{
		"github.com":     "github",
		"gitlab.com":     "gitlab",
		"dribbble.com":   "dribbble",
		"behance.net":    "behance",
		"medium.com":     "medium",
		"dev.to":         "dev.to",
		"hashnode.com":   "hashnode",
		"stackoverflow":  "stackoverflow",
		"linkedin.com":   "linkedin",
		"twitter.com":    "twitter",
		"x.com":          "x",
	}
	for host, name := range platforms {
		if strings.Contains(lower, host) {
			return name
		}
	}
	return "web"
}

func generateOTP() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := (int(b[0])<<24|int(b[1])<<16|int(b[2])<<8|int(b[3])) % 1_000_000
	if n < 0 {
		n = -n
	}
	return fmt.Sprintf("%06d", n), nil
}

func sendOTPEmail(to, otp string, cfg *config.Config) error {
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Work Email Verification Code\r\n\r\n"+
			"Your verification code is: %s\r\n"+
			"This code expires in 10 minutes.\r\n",
		cfg.SMTPFrom, to, otp,
	)
	auth := smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
	return smtp.SendMail(
		fmt.Sprintf("%s:%s", cfg.SMTPHost, cfg.SMTPPort),
		auth, cfg.SMTPFrom, []string{to}, []byte(msg),
	)
}

// ── LinkedIn HTTP helpers ─────────────────────────────────────────────────────

type linkedInTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type linkedInProfile struct {
	ID          string `json:"id"`
	DisplayName string `json:"localizedFirstName"`
	Headline    string `json:"headline"`
}

func exchangeLinkedInCode(ctx context.Context, code string, cfg *config.Config) (string, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {cfg.LinkedInRedirectURI},
		"client_id":     {cfg.LinkedInClientID},
		"client_secret": {cfg.LinkedInClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.linkedin.com/oauth/v2/accessToken",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LinkedIn token endpoint returned HTTP %d", resp.StatusCode)
	}

	var tok linkedInTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func fetchLinkedInProfile(ctx context.Context, token string) (*linkedInProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.linkedin.com/v2/me?projection=(id,localizedFirstName,localizedLastName,headline)",
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LinkedIn profile endpoint returned HTTP %d", resp.StatusCode)
	}

	var profile linkedInProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}
	return &profile, nil
}
