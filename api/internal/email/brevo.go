package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// BrevoService handles sending emails via the Brevo API
type BrevoService struct {
	apiKey      string
	senderEmail string
	senderName  string
	logger      *zap.Logger
	httpClient  *http.Client
	apiBaseURL  string
}

// NewBrevoService creates a new Brevo email service
func NewBrevoService(apiKey, senderEmail, senderName string, logger *zap.Logger) *BrevoService {
	return &BrevoService{
		apiKey:      apiKey,
		senderEmail: senderEmail,
		senderName:  senderName,
		logger:      logger,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		apiBaseURL:  "https://api.brevo.com/v3",
	}
}

type brevoSender struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type brevoRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoEmailRequest struct {
	Sender      brevoSender      `json:"sender"`
	To          []brevoRecipient `json:"to"`
	Subject     string           `json:"subject"`
	HTMLContent string           `json:"htmlContent"`
}

// SendEmail sends an email via the Brevo API
func (s *BrevoService) SendEmail(ctx context.Context, to, subject, htmlContent string) error {
	payload := brevoEmailRequest{
		Sender: brevoSender{
			Name:  s.senderName,
			Email: s.senderEmail,
		},
		To: []brevoRecipient{
			{Email: to},
		},
		Subject:     subject,
		HTMLContent: htmlContent,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiBaseURL+"/smtp/email", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("api-key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		io.Copy(io.Discard, resp.Body)
		s.logger.Info("Email sent",
			zap.String("to", to),
			zap.String("subject", subject),
		)
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("brevo API returned HTTP %d: %s", resp.StatusCode, string(respBody))
}

// SendUserInvite sends an invite email to a new user
func (s *BrevoService) SendUserInvite(ctx context.Context, toEmail, inviterName, inviteCode, appURL string) error {
	signupURL := fmt.Sprintf("%s/signup?code=%s", appURL, inviteCode)
	subject := fmt.Sprintf("%s invited you to TaskAI", inviterName)

	html := buildEmailTemplate(
		"You're Invited to TaskAI",
		fmt.Sprintf("<strong>%s</strong> has invited you to join <strong>TaskAI</strong>, an AI-native project management platform.", inviterName),
		signupURL,
		"Accept Invite",
		"This invite link will expire in 7 days.",
	)

	return s.SendEmail(ctx, toEmail, subject, html)
}

// SendProjectInvitation sends a project invitation email to an existing user with a one-click accept link
func (s *BrevoService) SendProjectInvitation(ctx context.Context, toEmail, inviterName, projectName, acceptToken, appURL string) error {
	acceptURL := fmt.Sprintf("%s/accept-invite?token=%s", appURL, acceptToken)
	subject := fmt.Sprintf("You've been invited to %s", projectName)

	html := buildEmailTemplate(
		fmt.Sprintf("Join \"%s\"", projectName),
		fmt.Sprintf("<strong>%s</strong> has invited you to collaborate on <strong>%s</strong> in TaskAI.", inviterName, projectName),
		acceptURL,
		"Accept Invitation",
		"This invitation link will expire in 7 days.",
	)

	return s.SendEmail(ctx, toEmail, subject, html)
}

// SendProjectInvitationNewUser sends a project invitation email to a user who needs to sign up first, with a one-click accept link
func (s *BrevoService) SendProjectInvitationNewUser(ctx context.Context, toEmail, inviterName, projectName, acceptToken, appURL string) error {
	acceptURL := fmt.Sprintf("%s/accept-invite?token=%s", appURL, acceptToken)
	subject := fmt.Sprintf("%s invited you to %s on TaskAI", inviterName, projectName)

	html := buildEmailTemplate(
		fmt.Sprintf("Join \"%s\" on TaskAI", projectName),
		fmt.Sprintf("<strong>%s</strong> has invited you to collaborate on <strong>%s</strong>. Create your TaskAI account to get started.", inviterName, projectName),
		acceptURL,
		"Accept Invitation",
		"This invitation link will expire in 7 days.",
	)

	return s.SendEmail(ctx, toEmail, subject, html)
}

// SendProjectMemberInvitation sends a notification to an existing user that they've been invited to a project
// SendTeamMemberAdded notifies a user that they have been added directly to a team.
func (s *BrevoService) SendTeamMemberAdded(ctx context.Context, toEmail, inviterName, teamName, appURL string) error {
	dashboardURL := appURL + "/app"
	subject := fmt.Sprintf("%s added you to \"%s\"", inviterName, teamName)

	html := buildEmailTemplate(
		fmt.Sprintf("You've joined \"%s\"", teamName),
		fmt.Sprintf("<strong>%s</strong> has added you to <strong>%s</strong> in TaskAI. You can start collaborating right away.", inviterName, teamName),
		dashboardURL,
		"Go to Dashboard",
		"You are now an active member of this team.",
	)

	return s.SendEmail(ctx, toEmail, subject, html)
}

func (s *BrevoService) SendProjectMemberInvitation(ctx context.Context, toEmail, inviterName, projectName, appURL string) error {
	settingsURL := appURL + "/app/settings"
	subject := fmt.Sprintf("%s invited you to join \"%s\"", inviterName, projectName)

	html := buildEmailTemplate(
		fmt.Sprintf("You're invited to \"%s\"", projectName),
		fmt.Sprintf("<strong>%s</strong> has invited you to collaborate on <strong>%s</strong> in TaskAI. Visit your settings to accept or reject the invitation.", inviterName, projectName),
		settingsURL,
		"View Invitation",
		"This invitation will remain pending until you accept or reject it.",
	)

	return s.SendEmail(ctx, toEmail, subject, html)
}

// buildEmailTemplate generates a responsive HTML email with TaskAI branding
func buildEmailTemplate(heading, bodyText, ctaURL, ctaLabel, footerNote string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin:0;padding:0;background-color:#0f1117;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
  <table width="100%%" cellpadding="0" cellspacing="0" style="background-color:#0f1117;padding:40px 20px;">
    <tr>
      <td align="center">
        <table width="100%%" cellpadding="0" cellspacing="0" style="max-width:520px;background-color:#1a1d27;border-radius:12px;overflow:hidden;">
          <!-- Header -->
          <tr>
            <td style="background:linear-gradient(135deg,#6366f1,#8b5cf6);padding:32px 32px 24px;">
              <h1 style="margin:0;font-size:14px;font-weight:600;color:rgba(255,255,255,0.8);letter-spacing:1px;text-transform:uppercase;">TaskAI</h1>
            </td>
          </tr>
          <!-- Body -->
          <tr>
            <td style="padding:32px;">
              <h2 style="margin:0 0 16px;font-size:22px;font-weight:700;color:#f1f5f9;">%s</h2>
              <p style="margin:0 0 28px;font-size:15px;line-height:1.6;color:#94a3b8;">%s</p>
              <table cellpadding="0" cellspacing="0" style="margin:0 0 28px;">
                <tr>
                  <td style="background-color:#6366f1;border-radius:8px;">
                    <a href="%s" style="display:inline-block;padding:14px 32px;font-size:15px;font-weight:600;color:#ffffff;text-decoration:none;">%s</a>
                  </td>
                </tr>
              </table>
              <p style="margin:0;font-size:13px;color:#64748b;">%s</p>
            </td>
          </tr>
          <!-- Footer -->
          <tr>
            <td style="padding:20px 32px;border-top:1px solid #2d3041;">
              <p style="margin:0;font-size:12px;color:#475569;">Sent by TaskAI. If you didn't expect this, you can safely ignore it.</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`, heading, bodyText, ctaURL, ctaLabel, footerNote)
}
