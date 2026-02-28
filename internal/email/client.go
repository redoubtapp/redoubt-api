package email

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v2"

	"github.com/redoubtapp/redoubt-api/internal/config"
)

// Client handles email sending via Resend.
type Client struct {
	client      *resend.Client
	fromAddress string
	fromName    string
	baseURL     string
}

// NewClient creates a new email client.
func NewClient(cfg config.EmailConfig, baseURL string) *Client {
	return &Client{
		client:      resend.NewClient(cfg.APIKey),
		fromAddress: cfg.FromAddress,
		fromName:    cfg.FromName,
		baseURL:     baseURL,
	}
}

// SendVerificationEmail sends an email verification link to the user.
func (c *Client) SendVerificationEmail(ctx context.Context, to, username, token string) error {
	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", c.baseURL, token)

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Verify Your Email</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; padding: 20px; max-width: 600px; margin: 0 auto;">
    <h1 style="color: #333;">Welcome to Redoubt, %s!</h1>
    <p style="color: #666; font-size: 16px; line-height: 1.5;">
        Thank you for registering. Please verify your email address by clicking the button below.
    </p>
    <p style="margin: 30px 0;">
        <a href="%s" style="background-color: #5865F2; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; font-weight: bold;">
            Verify Email Address
        </a>
    </p>
    <p style="color: #999; font-size: 14px;">
        If you didn't create an account, you can safely ignore this email.
    </p>
    <p style="color: #999; font-size: 14px;">
        This link will expire in 24 hours.
    </p>
    <hr style="border: none; border-top: 1px solid #eee; margin: 30px 0;">
    <p style="color: #999; font-size: 12px;">
        If the button doesn't work, copy and paste this link into your browser:<br>
        <a href="%s" style="color: #5865F2;">%s</a>
    </p>
</body>
</html>
`, username, verifyURL, verifyURL, verifyURL)

	_, err := c.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    fmt.Sprintf("%s <%s>", c.fromName, c.fromAddress),
		To:      []string{to},
		Subject: "Verify your Redoubt account",
		Html:    html,
	})

	return err
}

// SendPasswordResetEmail sends a password reset link to the user.
func (c *Client) SendPasswordResetEmail(ctx context.Context, to, username, token string) error {
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", c.baseURL, token)

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Reset Your Password</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; padding: 20px; max-width: 600px; margin: 0 auto;">
    <h1 style="color: #333;">Password Reset Request</h1>
    <p style="color: #666; font-size: 16px; line-height: 1.5;">
        Hi %s, we received a request to reset your password. Click the button below to choose a new password.
    </p>
    <p style="margin: 30px 0;">
        <a href="%s" style="background-color: #5865F2; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; font-weight: bold;">
            Reset Password
        </a>
    </p>
    <p style="color: #999; font-size: 14px;">
        If you didn't request a password reset, you can safely ignore this email. Your password will not be changed.
    </p>
    <p style="color: #999; font-size: 14px;">
        This link will expire in 1 hour.
    </p>
    <hr style="border: none; border-top: 1px solid #eee; margin: 30px 0;">
    <p style="color: #999; font-size: 12px;">
        If the button doesn't work, copy and paste this link into your browser:<br>
        <a href="%s" style="color: #5865F2;">%s</a>
    </p>
</body>
</html>
`, username, resetURL, resetURL, resetURL)

	_, err := c.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    fmt.Sprintf("%s <%s>", c.fromName, c.fromAddress),
		To:      []string{to},
		Subject: "Reset your Redoubt password",
		Html:    html,
	})

	return err
}

// SendWelcomeEmail sends a welcome email after successful verification.
func (c *Client) SendWelcomeEmail(ctx context.Context, to, username string) error {
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Welcome to Redoubt</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; padding: 20px; max-width: 600px; margin: 0 auto;">
    <h1 style="color: #333;">You're all set, %s!</h1>
    <p style="color: #666; font-size: 16px; line-height: 1.5;">
        Your email has been verified and your Redoubt account is now active. You can now log in and start using the platform.
    </p>
    <p style="margin: 30px 0;">
        <a href="%s" style="background-color: #5865F2; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; font-weight: bold;">
            Open Redoubt
        </a>
    </p>
    <hr style="border: none; border-top: 1px solid #eee; margin: 30px 0;">
    <p style="color: #999; font-size: 12px;">
        Thanks for joining Redoubt!
    </p>
</body>
</html>
`, username, c.baseURL)

	_, err := c.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    fmt.Sprintf("%s <%s>", c.fromName, c.fromAddress),
		To:      []string{to},
		Subject: "Welcome to Redoubt!",
		Html:    html,
	})

	return err
}
