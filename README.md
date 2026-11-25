# pagewatch

Get an alert on your phone when certain words appear on a web page.

pagewatch checks the page every 10 minutes, 30 minutes or hour (your choice). When one of your
words shows up, it alerts you by **phone notification**, **text message (SMS)**, **WhatsApp** or
**email**. Each word triggers one alert when it first appears, not one at every check.

It works on **Windows** and **Mac**. There is nothing else to install.

---

## Get started (about 5 minutes)

### Windows

1. Download **[pagewatch-windows.exe](https://github.com/alonazar/pagewatch/releases/latest/download/pagewatch-windows.exe)**.
2. Double-click the downloaded file.
   - If Windows shows **"Windows protected your PC"**, click **More info**, then **Run anyway**.
     (This appears because pagewatch is a small free program, not a paid, signed one.)
3. Answer the questions in the window that opens.

To change settings later, open the **Start menu** and type **pagewatch**.

### Mac

1. Download **[pagewatch-mac.zip](https://github.com/alonazar/pagewatch/releases/latest/download/pagewatch-mac.zip)**.
   Your Mac unzips it automatically. If it doesn't, double-click the zip file.
2. Double-click **pagewatch**. The first time, macOS will say it can't verify the developer:
   - Click **Done** (or **OK**).
   - Open **System Settings > Privacy & Security**, scroll down and click **Open Anyway**
     next to the message about pagewatch, then confirm.
   - Double-click **pagewatch** again.
3. Answer the questions in the Terminal window that opens.

To change settings later, press **Cmd+Space** and type **pagewatch**.
macOS may show a "Background Items Added" notice; that's the regular check.

---

## What the setup asks

1. **The web page** to watch. Copy its address from your browser's address bar and paste it.
2. **The words** to look for, separated by commas, for example `tickets available, sold out`.
   Upper and lower case don't matter.
3. **How often to check:** every 10 minutes, every 30 minutes or every hour. You can change it later.
4. **How to alert you:**

| Option | What you need | Cost |
|---|---|---|
| **Phone notification** (easiest) | The free **ntfy** app ([iPhone](https://apps.apple.com/app/ntfy/id1625396347), [Android](https://play.google.com/store/apps/details?id=io.heckel.ntfy)). No account. | Free |
| **Text message (SMS)** | An upgraded (paid) [Twilio](https://www.twilio.com/try-twilio) account and a Twilio phone number. A free trial won't work. | A few cents per message |
| **WhatsApp** | An upgraded (paid) Twilio account, a WhatsApp sender and a message template approved by Meta (see below) | Per message, set by Meta and Twilio |
| **Email** | A Gmail account with an [App Password](https://myaccount.google.com/apppasswords) | Free |

pagewatch sends a test alert so you know it works. You can then add another way to get alerts,
for example a phone notification *and* an email. Every alert goes to all of them.

### Notes on SMS (Twilio)
- **A free trial Twilio account won't work.** Twilio trials can only send Twilio's own preset
  messages, not custom alerts like pagewatch's. In the Twilio Console, click **Upgrade** and add a
  payment method first. If you'd rather not pay, use the free phone notification (ntfy).
- Sign up at twilio.com, then copy the **Account SID** and **Auth Token** from the Console home page.
- Get a phone number in the Console (**Phone Numbers > Buy a number**).
- **United States:** carriers require the sending number to be registered first
  (Twilio walks you through "Toll-free verification"). This can take a few days.
- **Texting another country:** in the Twilio Console, open **Messaging > Settings > Geo permissions**
  and tick that country, or Twilio will refuse to send there.

### Notes on WhatsApp (Twilio)
WhatsApp needs more one-time setup than SMS, all in the Twilio Console:

1. An **upgraded (paid) Twilio account**.
2. A **WhatsApp sender**: under **Numbers & senders > WhatsApp**, register a Twilio phone number
   that isn't already used with WhatsApp. Twilio asks you to connect a Meta (Facebook) Business account.
3. A **message template approved by Meta**: WhatsApp only lets businesses start a conversation
   with an approved template. Under **Messaging > Templates**, create a template with category
   **Utility** for WhatsApp, with exactly this text:

   ```
   pagewatch alert: {{1}}. Open the page to see more.
   ```

   Submit it for approval. Once approved, copy its **Content SID** (starts with `HX`) into pagewatch's setup.

**Just trying it out?** Use Twilio's WhatsApp Sandbox ("Send a WhatsApp message" on the Console
home page) and leave the Content SID empty. Sandbox alerts only arrive within 24 hours of the
receiving phone last messaging the sandbox, so it isn't suitable for real alerts.

---

## Good to know

- **Your computer has to be on** (it can be locked or asleep). If it was off or asleep,
  pagewatch checks again soon after it wakes up.
- **Pages built with JavaScript:** pagewatch reads a page's text without running its scripts.
  If a word you can see in your browser is never found, the page probably loads that text
  with JavaScript, and pagewatch can't see it.
- **Pages that need a login** aren't supported.
- **Hebrew, Arabic and similar languages:** words are matched as whole words, so `מבצע`
  won't match `והמבצע`. To match the word anywhere, set `whole_words: false` in the settings file.

## The menu

When you open pagewatch after setting it up, you can:

- **Check now**
- **Send a test alert**
- **Show current settings**: pages, words, how often, whether the schedule is running, the last
  check, and where alerts go. Passwords and auth tokens are never shown, only whether they're set.
- **Change how often to check**: every 10 minutes, 30 minutes or hour. Takes effect right away.
- **Add an alert method**: get alerts in more than one way, for example SMS *and* email.
  Each new method gets its own test alert before it's saved.
- **Remove an alert method** (you always keep at least one)
- **Change settings**: pages, words or how you're alerted
- **Show recent activity**: what each check found
- **Stop watching and remove pagewatch**

## Settings file (advanced)

All settings are in a text file you can edit:

- Windows: `%AppData%\pagewatch\config.yaml`
- Mac: `~/Library/Application Support/pagewatch/config.yaml`

There you can watch several pages, turn on more than one alert method, change how often
checks run (`check_every_minutes`, any value from 5 to 1440 minutes), or use a different email
server. After changing `check_every_minutes` in the file, open pagewatch once so the schedule is updated.

The settings file contains your alert passwords and tokens. It is stored only on your computer.

## Removing pagewatch

Open pagewatch and choose **Stop watching and remove pagewatch**. Then delete the file you downloaded.

## Problems?

- **No alert arrives:** open pagewatch, choose **Send a test alert** and read any error message.
- **Check what happened:** choose **Show recent activity**.
- **"Windows protected your PC" or "can't be opened" on Mac:** see the steps under *Get started*.
- **Antivirus says pagewatch is a threat:** older versions aren't code-signed (see
  *Code signing policy* below), and pagewatch does things that
  some antivirus programs treat as suspicious in unknown programs: it copies itself to your
  settings folder and creates a scheduled task to run the checks. Its full source code is in
  this repository. Please don't turn off your antivirus. Instead:
  - Report it to your antivirus maker as a false positive. For Windows Security (Microsoft
    Defender), use https://www.microsoft.com/wdsi/filesubmission and choose "Incorrectly detected".
  - [Open an issue](https://github.com/alonazar/pagewatch/issues) with the threat name shown,
    so we can look into it.
- Still stuck? [Open an issue](https://github.com/alonazar/pagewatch/issues).

---

## For developers

pagewatch is a single Go program with one dependency (`gopkg.in/yaml.v3`).

```sh
go test ./...
go run . check        # run one check with your saved settings
go run .              # menu / setup
```

- Windows release builds use `-ldflags -H=windowsgui`, so the scheduled check runs without
  flashing a window. When opened by the user, the program creates its own console window.
- The schedule uses Windows Task Scheduler (task "pagewatch") or a launchd agent on Mac
  (`~/Library/LaunchAgents/io.github.pagewatch.plist`).
- On first run, pagewatch copies itself into the settings folder, so the schedule keeps
  working if the downloaded file is moved or deleted.

**Releasing:** push a tag such as `v1.0.0`. GitHub Actions tests the code, builds
`pagewatch-windows.exe` and a universal `pagewatch-mac.zip`, sends the Windows file for
signing (see below), and publishes them as a release.

## Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io), certificate by
[SignPath Foundation](https://signpath.org).

Signing starts with the first release after SignPath Foundation approves the project.
Releases up to and including v0.1.7 are not signed.

- Only `pagewatch-windows.exe` is signed. It is built by GitHub Actions from the tagged source
  in this repository ([release.yml](.github/workflows/release.yml)); nothing is signed from a
  personal computer.
- Every signing request is approved by hand.

**Team roles**

| Role | Who |
| --- | --- |
| Committers and reviewers | [Alon Azar](https://github.com/alonazar) |
| Approvers | [Alon Azar](https://github.com/alonazar) |

**Privacy policy:** this program will not transfer any information to other networked
systems unless specifically requested by the user or the person installing or operating it.
pagewatch only downloads the web pages you list in its settings, and only sends alerts to the
services you set up (ntfy, Twilio for SMS or WhatsApp, or your email provider). Those services'
own privacy policies apply to what you send through them. pagewatch has no analytics, no
tracking and no server of its own.

## License

MIT
