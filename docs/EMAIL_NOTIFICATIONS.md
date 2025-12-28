# Email Notifications Setup

## Overview

Secure File Drop supports email notifications for important events:
- **Email Verification**: Sent when users register
- **Password Reset**: Sent when users request password reset
- **File Upload Complete**: Sent when a file upload is successfully processed
- **File Downloaded**: Sent when someone downloads your file
- **File Deleted**: Sent when a file is manually deleted by admin

## Configuration

Email notifications are controlled via environment variables:

### Required Variables (when email is enabled)

```bash
SFD_EMAIL_ENABLED=true              # Set to "true" to enable email notifications
SFD_SMTP_HOST=smtp.gmail.com        # Your SMTP server hostname
SFD_SMTP_PORT=587                   # SMTP server port (default: 587 for TLS)
SFD_SMTP_USER=your-email@gmail.com  # SMTP username (usually your email)
SFD_SMTP_PASSWORD=your-app-password # SMTP password or app-specific password
SFD_FROM_EMAIL=noreply@yourdomain.com # From address (defaults to SMTP_USER)
SFD_BASE_URL=https://yourdomain.com # Base URL for links in emails
```

### Example: Gmail Configuration

1. Enable 2-factor authentication on your Gmail account
2. Generate an app-specific password: https://myaccount.google.com/apppasswords
3. Use the following configuration:

```bash
SFD_EMAIL_ENABLED=true
SFD_SMTP_HOST=smtp.gmail.com
SFD_SMTP_PORT=587
SFD_SMTP_USER=your-email@gmail.com
SFD_SMTP_PASSWORD=your-16-char-app-password
SFD_FROM_EMAIL=noreply@gmail.com
SFD_BASE_URL=http://localhost:8080
```

### Example: SendGrid Configuration

```bash
SFD_EMAIL_ENABLED=true
SFD_SMTP_HOST=smtp.sendgrid.net
SFD_SMTP_PORT=587
SFD_SMTP_USER=apikey
SFD_SMTP_PASSWORD=your-sendgrid-api-key
SFD_FROM_EMAIL=noreply@yourdomain.com
SFD_BASE_URL=https://yourdomain.com
```

### Example: AWS SES Configuration

```bash
SFD_EMAIL_ENABLED=true
SFD_SMTP_HOST=email-smtp.us-east-1.amazonaws.com
SFD_SMTP_PORT=587
SFD_SMTP_USER=your-ses-smtp-username
SFD_SMTP_PASSWORD=your-ses-smtp-password
SFD_FROM_EMAIL=verified-sender@yourdomain.com
SFD_BASE_URL=https://yourdomain.com
```

## Disabling Email Notifications

If `SFD_EMAIL_ENABLED` is not set to "true" (or is omitted), email notifications are disabled. The system will log email events to the console instead:

```
EMAIL (disabled): To: user@example.com, Subject: File Upload Complete
```

This is useful for:
- Development environments
- Testing without sending actual emails
- Deployments where email is not required

## Email Templates

All emails are sent as HTML with responsive design and include:
- Clear subject lines
- Branded header with Secure File Drop colors
- Action buttons (when applicable)
- Fallback plain text links
- Footer with security notes

### Sample Emails

**Verification Email:**
```
Subject: Verify Your Email - Secure File Drop
Content: Click to verify your email address with a prominent blue button
Expires: Token valid until email is verified
```

**Upload Complete:**
```
Subject: File Upload Complete - Secure File Drop
Content: Shows filename, file ID, and link to dashboard
```

**Download Notification:**
```
Subject: File Downloaded - Secure File Drop
Content: Shows filename and IP address of downloader
```

## Testing Email Configuration

1. Start the backend with email enabled:
   ```bash
   docker compose up -d
   ```

2. Register a new user - you should receive a verification email

3. Check backend logs for email status:
   ```bash
   docker compose logs backend | grep EMAIL
   ```

4. Successful send:
   ```
   EMAIL SENT: To: user@example.com, Subject: Verify Your Email
   ```

5. Error (e.g., bad credentials):
   ```
   EMAIL ERROR: Failed to send to user@example.com: authentication failed
   ```

## Security Notes

- **Never commit SMTP passwords to version control**
- Use app-specific passwords when available (Gmail, etc.)
- Consider using environment-specific `.env` files
- Verify sender email addresses with your SMTP provider
- Use TLS/STARTTLS (port 587) for secure transmission
- Rotate SMTP credentials periodically

## Troubleshooting

### Email not sending

1. Check `SFD_EMAIL_ENABLED=true` is set
2. Verify SMTP credentials are correct
3. Check backend logs for error messages
4. Ensure SMTP host/port are accessible from container
5. Verify sender email is authorized with your SMTP provider

### Gmail "Less secure app" error

- Gmail requires app-specific passwords with 2FA enabled
- Don't use your regular Gmail password
- Generate an app password at: https://myaccount.google.com/apppasswords

### SendGrid authentication failed

- Ensure username is exactly "apikey"
- Verify API key has "Mail Send" permissions
- Check sender email is verified in SendGrid dashboard

### AWS SES errors

- Verify sender email in SES console
- If in sandbox mode, verify recipient emails too
- Check SMTP credentials are from SES (not IAM)
- Ensure region in SMTP host matches your SES region

## Production Recommendations

1. **Use a dedicated SMTP service**: SendGrid, Mailgun, AWS SES
2. **Set up SPF/DKIM**: Configure DNS records to prevent spoofing
3. **Monitor email delivery**: Track bounces and delivery failures
4. **Use a custom domain**: More professional than Gmail
5. **Set appropriate FROM address**: noreply@yourdomain.com
6. **Configure SFD_BASE_URL**: Use your actual domain, not localhost

## Future Enhancements

Potential improvements for email notifications:

- [ ] File expiration warnings (24 hours before expiry)
- [ ] Scheduled digest emails (daily/weekly activity summary)
- [ ] Email templates stored in database (admin customizable)
- [ ] Internationalization (multi-language email support)
- [ ] Unsubscribe links for notification preferences
- [ ] Email delivery status tracking
- [ ] Custom email branding (logo, colors)
- [ ] Rate limiting on notification emails
