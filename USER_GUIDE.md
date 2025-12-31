# Secure File Drop - User Guide

## What is Secure File Drop?

Secure File Drop is a **self-hosted file sharing service** that allows you to safely upload and share files with others. Think of it like a private version of WeTransfer or Dropbox, but one that **you control completely** on your own server.

Unlike public file sharing services where your files are stored on someone else's servers, Secure File Drop runs on **your own infrastructure**, giving you complete control over your data, privacy, and security.

## Who is this for?

**Perfect for:**
- Businesses that need to share sensitive documents securely
- Healthcare providers sharing patient information (HIPAA-compliant when properly configured)
- Legal firms exchanging confidential files with clients
- Freelancers and contractors sharing work files
- Teams that want private, secure file sharing without monthly subscriptions
- Anyone who values privacy and wants control over their data

**Not for you if:**
- You just want to share vacation photos with friends (use Google Photos or similar)
- You don't have access to a server or don't want to manage one
- You need unlimited storage (you're limited by your server's disk space)

---

## How Does It Work?

### The Basic Flow

```
1. You (Admin) → Upload File
2. System → Stores file securely
3. System → Generates unique, expiring download link
4. You → Share link with recipient
5. Recipient → Downloads file before link expires
6. System → (Optional) Automatically deletes file after expiration
```

### What Happens Behind the Scenes?

#### 1. **File Upload**
When you upload a file:
- The file is **encrypted** and stored in a secure storage system (MinIO)
- A **unique identifier** is created for your file
- The system records who uploaded it, when, and how large it is
- Progress tracking shows you the upload status in real-time

#### 2. **Secure Storage**
Your files are stored:
- In an **object storage system** (like Amazon S3, but private)
- With **metadata** in a database (PostgreSQL) for tracking
- Completely **separated from the internet** until you share them
- With **automatic expiration** settings you can control

#### 3. **Sharing**
When you share a file:
- A **time-limited, signed URL** is generated (like a temporary key)
- You can optionally add a **password** for extra security
- You get a **QR code** that recipients can scan with their phone
- An **email notification** can be sent to the recipient (if configured)

#### 4. **Downloading**
When someone downloads your file:
- They **must have the exact link** you sent them (can't guess it)
- The link **expires** after a set time (you choose: hours, days, weeks)
- You can see **who downloaded** it and when (audit trail)
- The link becomes invalid after expiration

#### 5. **Cleanup**
The system automatically:
- **Deletes expired files** to save space
- **Backs up the database** daily (so you don't lose tracking info)
- **Logs all activities** for security auditing
- **Monitors system health** and alerts you if something's wrong

---

## Key Features Explained (Non-Technical)

### 🔐 Security & Privacy

#### **Password Protection**
- You can require a password for downloads
- Like putting a lock on your shared folder
- Only people with both the link AND password can access

#### **Automatic Expiration**
- Files automatically become unavailable after a set time
- Like self-destructing messages, but for files
- Prevents old sensitive files from lingering forever

#### **Account Lockout**
- After 5 failed login attempts, the account locks for 15 minutes
- Prevents hackers from guessing passwords repeatedly
- Protects your admin account from brute-force attacks

#### **Encrypted Passwords**
- Your admin password is stored as an unreadable hash
- Even if someone steals the database, they can't see your password
- Uses industry-standard bcrypt encryption

#### **Audit Logging**
- Every action is recorded: uploads, downloads, deletions, logins
- Like a security camera, but for file activity
- Helps you track who did what and when

### 📊 Monitoring & Reliability

#### **Real-Time Dashboard (Grafana)**
- Visual charts showing upload/download activity
- Storage usage graphs (how much space you're using)
- System health indicators (is everything working?)
- Think of it as your application's "dashboard" like in a car

#### **Automated Backups**
- Database is automatically backed up every day
- Keeps last 7 days of backups (configurable)
- Like having a time machine for your file tracking data
- Backups are compressed to save space

#### **Health Monitoring (Prometheus)**
- Constantly checks if all parts are working
- Alerts you if something breaks
- Tracks performance metrics (speed, errors, etc.)
- Like a doctor monitoring your application's vital signs

#### **Circuit Breakers**
- Automatically stops trying to use broken services
- Prevents one problem from cascading into many
- Self-heals when the issue is resolved
- Like a electrical circuit breaker in your house

### 📁 File Management

#### **Multi-File Upload**
- Upload multiple files at once (not one-by-one)
- Drag and drop from your computer
- Real-time progress bars for each file
- Resume interrupted uploads (doesn't lose progress)

#### **File Type Detection**
- Automatically shows icons based on file type
- PDF, Word, Excel, images, videos all get proper icons
- Makes it easy to recognize files at a glance

#### **Storage Quotas**
- Each user has a storage limit (prevents abuse)
- See how much space you've used
- Get warnings when approaching the limit
- Admin can adjust quotas per user

#### **Automatic Cleanup**
- Old files in "pending" or "failed" states are removed
- Runs automatically every hour (configurable)
- Keeps your storage clean and organized
- No manual maintenance required

### 🔔 Notifications

#### **Email Alerts**
When configured, the system can email you about:
- New file uploads
- File downloads (who downloaded what)
- File deletions
- Security events (failed logins, account lockouts)
- Backup failures
- Think of it as your application sending you updates

#### **Security Notifications**
Automatic emails for:
- Multiple failed login attempts
- Account being locked out
- Password changes
- Suspicious activity

---

## How to Use Secure File Drop

### For Admins (You)

#### **First Time Setup**
1. **Log in** with your admin username and password
2. **Configure settings** (optional email notifications, storage limits)
3. **Create user accounts** (if multiple people will upload files)
4. **Test upload** a file to make sure everything works

#### **Uploading Files**
1. Click **"Upload Files"** or drag files to the upload area
2. Select one or more files from your computer
3. Watch the progress bar as files upload
4. When complete, you'll see your file listed

#### **Sharing Files**
1. Find your uploaded file in the file list
2. Click **"Get Download Link"**
3. Optionally set:
   - **Expiration time** (how long link is valid)
   - **Password** (extra security)
   - **Download limit** (max number of downloads)
4. **Copy the link** and share it (email, chat, etc.)
5. Optionally scan the **QR code** on your phone

#### **Monitoring Activity**
1. Go to **Dashboard** to see:
   - Recent uploads/downloads
   - Storage usage
   - Active files
2. Check **Grafana** (port 3000) for detailed metrics
3. View **Audit Logs** to see who did what

#### **Managing Files**
- **View all files** in the file list
- **Search** by filename, uploader, or date
- **Delete** files manually if needed
- **Check download count** for each file

### For Recipients (People You Share With)

#### **Downloading a File**
1. Click the link you received
2. If password-protected, enter the password
3. Click **"Download"**
4. File downloads to your computer
5. That's it!

#### **Using QR Codes**
1. Open your phone's camera app
2. Point at the QR code
3. Tap the notification that appears
4. Enter password if required
5. Download the file on your phone

---

## Common Use Cases

### **1. Sharing Documents with Clients**

**Scenario:** You're a lawyer sending a contract to a client

**How to do it:**
1. Upload the contract PDF
2. Set expiration to 7 days
3. Add a password (tell client separately)
4. Send the link via email
5. System emails you when client downloads it
6. File auto-deletes after 7 days

**Why it's secure:**
- Client must have both link and password
- Link expires after 7 days
- You know exactly when they downloaded it
- File doesn't stay on your server forever

---

### **2. Collecting Files from Multiple People**

**Scenario:** You need employees to submit expense reports

**How to do it:**
1. Create user accounts for each employee
2. Give them login credentials
3. They log in and upload their reports
4. You see all uploads in your admin dashboard
5. Download reports when ready
6. Files auto-delete after you've processed them

**Why it's useful:**
- Everyone has their own account (accountability)
- You see who uploaded what and when
- No email attachments to manage
- Automatic organization and cleanup

---

### **3. Temporary File Sharing**

**Scenario:** You need to share a large video file with a colleague

**How to do it:**
1. Upload the video (up to 50GB by default)
2. Set expiration to 24 hours
3. Share the link via Slack/Teams
4. Colleague downloads when ready
5. File auto-deletes next day

**Why it's better than email:**
- No file size limits (email usually caps at 25MB)
- Doesn't fill up recipient's inbox
- Auto-cleanup saves storage
- Progress tracking for large files

---

### **4. Secure Medical Records Transfer**

**Scenario:** Doctor sharing patient test results with specialist

**How to do it:**
1. Upload patient records (X-rays, lab results)
2. Set password (patient's date of birth)
3. Set expiration to 48 hours
4. Email link to specialist
5. Specialist downloads with password
6. Records auto-delete after 48 hours

**Why it's HIPAA-friendly:**
- Password protection (encryption in transit)
- Audit logs (who accessed what)
- Automatic deletion (minimizes data retention)
- No third-party services (you control the data)

---

## Understanding System Components

### **What's Running When the System is On?**

Think of Secure File Drop as a small office with different departments:

#### **1. The Lobby (Web Interface)**
- What you see when you visit the website
- Handles login, file upload forms, download pages
- The "front desk" of your application
- Runs on port 8080

#### **2. The Vault (MinIO Storage)**
- Where actual files are stored
- Like a secure file cabinet
- Files are stored as "objects" with unique IDs
- Runs on port 9000

#### **3. The Logbook (PostgreSQL Database)**
- Records who uploaded what, when
- Tracks download counts, expiration dates
- Stores user accounts and settings
- Like the office's filing system for paperwork
- Runs on port 5432

#### **4. The Monitor (Prometheus)**
- Watches everything and collects statistics
- Counts uploads, downloads, errors
- Tracks system performance
- Like security cameras and sensors
- Runs on port 9090

#### **5. The Dashboard (Grafana)**
- Turns Prometheus data into pretty charts
- Shows graphs of activity over time
- Helps you see trends and patterns
- Like a TV showing security camera feeds
- Runs on port 3000

#### **6. The Receptionist (Traefik Proxy)**
- Routes traffic to the right place
- Handles HTTPS/SSL certificates
- Like a receptionist directing visitors
- Runs on ports 80 (HTTP) and 443 (HTTPS)

---

## Security Features in Plain English

### **How Your Data is Protected**

#### **1. Encryption at Rest**
- Files stored on disk are scrambled
- Only the system can unscramble them
- Like locking files in a safe

#### **2. Encryption in Transit**
- Data traveling over the internet is encrypted
- HTTPS protocol (the padlock in your browser)
- Like sending letters in locked boxes

#### **3. Access Control**
- Only logged-in users can upload
- Only people with links can download
- Like having ID badges to enter different rooms

#### **4. Rate Limiting**
- Limits how many requests someone can make
- Prevents system abuse and attacks
- Like a bouncer limiting how many people enter at once

Different limits for different actions:
- **Login attempts:** 10 per minute (prevents password guessing)
- **Uploads:** 20 per hour (prevents storage abuse)
- **Downloads:** 100 per hour (prevents bandwidth abuse)
- **Admin actions:** 50 per minute (admin has more freedom)

#### **5. Audit Trail**
- Every action is logged with:
  - Who did it (username or IP address)
  - What they did (uploaded, downloaded, deleted)
  - When they did it (timestamp)
  - Whether it succeeded or failed
- Logs are stored permanently for compliance
- Like having video recordings of all activity

---

## Frequently Asked Questions

### **Q: How big can files be?**
**A:** By default, up to 50GB per file. Your admin can change this limit in the configuration.

### **Q: How long do files stay available?**
**A:** As long as you set the expiration for. Could be 1 hour, 1 day, 1 week, 1 month, or forever. You decide when creating the download link.

### **Q: What happens if someone guesses my download link?**
**A:** Extremely unlikely - links contain random characters making them impossible to guess. Think of it like a 32-character password.

### **Q: Can I see who downloaded my file?**
**A:** Yes! The audit logs show every download with timestamp and IP address.

### **Q: What file types can I upload?**
**A:** Any file type - documents, images, videos, code, databases, etc. No restrictions.

### **Q: Is my data backed up?**
**A:** The database (tracking info) is backed up daily. The actual files are not automatically backed up - you should back up the storage volume separately.

### **Q: Can I revoke a download link?**
**A:** Yes, you can delete the file which invalidates all links to it.

### **Q: What if I forget my password?**
**A:** Currently, password reset must be done by the admin through the database. Future versions will have self-service password reset.

### **Q: How many users can I have?**
**A:** Unlimited, but each user needs storage quota. Total limited by your server's disk space.

### **Q: Can users upload files to each other?**
**A:** No - users upload to the system, then share download links. Files aren't "sent to" specific users.

### **Q: What happens if my server runs out of space?**
**A:** Uploads will fail with an error. The system warns you in the dashboard when space is low. Set up automatic cleanup to prevent this.

### **Q: Is this secure enough for [sensitive industry]?**
**A:** The application provides strong security features, but **you** are responsible for:
- Keeping the server secure (updates, firewall, etc.)
- Using strong passwords
- Enabling HTTPS
- Following your industry's compliance requirements
- Regular backups

For healthcare (HIPAA), legal, or financial use, consult with a compliance expert to ensure your deployment meets all requirements.

### **Q: Do I need to be technical to use this?**
**A:** To **use** it (upload/download): No, it's as easy as WeTransfer.  
To **set it up**: Yes, you need basic Linux and Docker knowledge, or hire someone to set it up for you.  
To **maintain** it: Basic - mostly just monitoring the dashboard and occasionally checking logs.

---

## What Makes This Different from [Other Service]?

### **vs. WeTransfer**
| Feature | Secure File Drop | WeTransfer |
|---------|------------------|------------|
| **Privacy** | Complete - you own the data | Files stored on their servers |
| **Cost** | One-time setup, then free | Free (limited) or paid subscription |
| **File Size** | Your server's limit (50GB+ possible) | 2GB free, 200GB paid |
| **Expiration** | You control | 7 days |
| **Branding** | Your own domain | WeTransfer branding |
| **Compliance** | You control and audit | Trust their compliance |

### **vs. Dropbox**
| Feature | Secure File Drop | Dropbox |
|---------|------------------|----------|
| **Storage** | Your server's capacity | 2GB free, paid tiers |
| **Privacy** | You control encryption keys | Dropbox has access |
| **Sharing** | Time-limited, one-time links | Persistent folders |
| **Cost** | Free after setup | Monthly subscription |
| **Sync** | No sync (one-time sharing) | Continuous sync |
| **Best for** | Sharing specific files temporarily | Long-term collaboration |

### **vs. Google Drive**
| Feature | Secure File Drop | Google Drive |
|---------|------------------|--------------|
| **Privacy** | Completely private | Google can scan files |
| **Integration** | Standalone | Integrates with Google services |
| **Collaboration** | Share only | Real-time collaboration |
| **Cost** | Free after setup | 15GB free, then paid |
| **Compliance** | Full control | Trust Google's compliance |
| **Best for** | Secure one-time sharing | Document collaboration |

---

## Tips for Best Use

### **For Maximum Security:**
1. ✅ Always use HTTPS (set up SSL certificates)
2. ✅ Set short expiration times (hours or days, not weeks)
3. ✅ Use passwords for sensitive files
4. ✅ Enable email notifications to track access
5. ✅ Review audit logs regularly
6. ✅ Keep the system updated
7. ✅ Use strong admin password (20+ characters)
8. ✅ Limit user accounts (only create for people who need them)

### **For Better Performance:**
1. ✅ Clean up old files regularly
2. ✅ Monitor storage usage in Grafana
3. ✅ Set automatic cleanup intervals
4. ✅ Use compression for text files
5. ✅ Check backup logs weekly

### **For Easier Management:**
1. ✅ Use descriptive filenames
2. ✅ Set consistent expiration policies
3. ✅ Create user accounts with clear names
4. ✅ Document your sharing procedures
5. ✅ Keep track of who you've shared with

---

## Summary

**Secure File Drop is your private, secure file sharing service.** 

It gives you:
- ✅ Complete control over your files and data
- ✅ Enterprise-grade security features
- ✅ Professional monitoring and logging
- ✅ Automatic backups and maintenance
- ✅ No monthly fees or storage limits (beyond your server)

**Think of it as:**
- Your own private WeTransfer
- A file sharing service you control
- A secure way to share sensitive documents
- An alternative to trusting third-party services

**Perfect when you need:**
- Privacy and control
- Compliance with regulations (HIPAA, GDPR, etc.)
- Custom expiration and access controls
- Audit trails for accountability
- No monthly costs
- Professional features without enterprise pricing

---

## Getting Help

- **Documentation:** See `/docs` folder for technical guides
- **Deployment Guide:** `docs/PRODUCTION_DEPLOYMENT.md`
- **Features List:** `docs/FEATURES.md`
- **Quick Start:** `QUICKSTART.md`
- **GitHub:** https://github.com/dreamingfree09/secure-file-drop

**Remember:** This is a self-hosted solution, which means **you** are responsible for security, backups, and maintenance. With great control comes great responsibility! 🚀
