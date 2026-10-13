# Android TV

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

Play Apple Music in nu11signal's terminal-style interface on your TV, using
only the remote. Install the APK, sign in once, then browse and play.

## Requirements

- An Android TV or Google TV device running **Android 9 or later** (API 28+).
  Tested on a **Google TV Streamer**.
- An **Apple Music subscription**, your Apple ID sign-in details, and access
  to any device needed for your verification code.
- An internet connection and a remote with arrows, OK and Back.

| Available | Not available in this version |
|-----------|-------------------------------|
| Apple Music catalog, library playlists, search and playback | Local music files on the TV |
| Background playback and remote media keys | Play Store distribution of nu11signal |
| Rain that follows the TV audio, with permission | A native TV interface: the app displays the terminal UI |

Kode Mono, the interface font, is included; there is no font to install.
Library-only songs such as uploads are listed but skipped during playback.

## Install the APK

Sideloading means installing an APK outside the Play Store. **USB debugging
is not needed** for the remote-control method below. Settings names vary
with the TV and software version.

### Download and check on a computer

1. Open [GitHub Releases](https://github.com/wahh-22/nu11signal/releases)
   and choose a release with Android TV assets.
2. Download **both** `nu11signal-tv-<version>.apk` and
   `nu11signal-tv-<version>.apk.sha256` into the same folder.
3. In that folder, verify the checksum. Replace `<version>` with the
   downloaded version:

   ```sh
   # macOS
   shasum -a 256 -c nu11signal-tv-<version>.apk.sha256

   # Linux
   sha256sum -c nu11signal-tv-<version>.apk.sha256
   ```

   On Windows, use PowerShell's
   `Get-FileHash .\nu11signal-tv-<version>.apk -Algorithm SHA256` and compare
   the hash with the one in the `.sha256` file (letter case does not matter).
   **Continue only if it matches**; the check commands above should say `OK`.
   If it does not, download the pair again; do not install the mismatched APK.

### Install with the remote (Google TV)

1. Install an APK downloader such as **Downloader** from the TV's Play
   Store. This is the installer, not nu11signal itself.
2. If installing unknown apps is unavailable, enable Developer options:
   open **Settings > System > About**, select **Android TV OS build** and
   press OK repeatedly (usually seven times) until Developer options are
   enabled. **Do not enable USB debugging** for this method.
3. Open **Settings > Apps > Special app access > Install unknown apps**
   and allow your downloader. On some TVs this is under **Security &
   restrictions > Unknown sources**, or the APK installer takes you there.
4. In the downloader, enter the direct APK link copied from the release:

   ```text
   https://github.com/wahh-22/nu11signal/releases/download/v<version>/nu11signal-tv-<version>.apk
   ```

   Use the same release you checked on the computer. Alternatively, transfer
   the checked APK to the TV and open it with a TV file manager; allow that
   file manager to install unknown apps instead.
5. Open the downloaded APK, choose **Install**, then **Open**. You can
   launch nu11signal later from the TV's apps list.
6. Turn off the install-unknown-apps allowance for the downloader/file
   manager once installation is finished.

### Advanced: install from a computer

If you already use Android Platform Tools and have authorized a debugging
connection to the TV, install the checked APK with:

```sh
adb install -r nu11signal-tv-<version>.apk
```

This optional method requires debugging setup; it is not needed for sign-in
or normal use. Disable debugging when you no longer need it.

## First launch and sign-in

1. Launch **nu11signal**. Keep the TV awake and connected. Loading can
   take **about 40 seconds** on a Google TV Streamer.
2. On an unsigned-in launch, a full-screen guide appears. Its exact text is:

   > Sign in to Apple Music
   >
   > nu11signal plays your Apple Music through Apple's web player. Sign in once on this TV; it stays signed in.
   >
   > 1. Use the arrows to move the white circle to Sign In (top right of the page), then press OK.
   > 2. In the Apple ID window, move the circle to each field and press OK to open the keyboard. Type your Apple ID, password and verification code.
   > 3. When you finish, nu11signal returns by itself.
   >
   > Press OK to start · Back to skip for now

3. Press **OK** to dismiss the guide. Move the white circle to Apple's
   **Sign In** control and press OK.
4. In Apple's sign-in window, move to each field and press **OK**. Use
   the TV keyboard to enter your Apple ID, password and verification code
   when requested, and follow Apple's prompts.
5. Wait for the terminal to return automatically. The banner
   **“Signed in to Apple Music”** appears for four seconds. Library loading
   can take another roughly 40 seconds while Apple switches to your account.

The Apple Music session **persists across app restarts**; you do not need to
sign in every time. Back on the guide skips sign-in for now.

| Remote during sign-in | Action |
|-----------------------|--------|
| Arrows | Move the white-circle pointer; holding moves faster |
| Up/down at the screen edge | Scroll the page |
| OK | Tap under the pointer; on a text field, open the TV keyboard |
| Arrows / OK while the keyboard is open | Select keyboard keys normally |
| Back | Dismiss the keyboard first; otherwise go back in the Apple ID window, then close it; outside that window, return to nu11signal |
| Long OK / media keys | No action in pointer mode |

## Browse and play with the remote

Select a library playlist and press **OK** to open it. Select its play row
or a track and press OK to play. For the catalog, press **Menu**, type a
search, hide the keyboard with Back, then move down to the matches. OK on
an artist opens its page; OK on a song plays it with the other listed songs
queued.

### Remote key reference

These actions apply when the TV keyboard is **hidden**. When it is open,
arrows and OK operate the keyboard instead.

| Remote key | Action |
|------------|--------|
| Up / down | Move through list rows. In SEARCH, move between the input and matches. Up past the first row/input focuses the navigation tabs |
| Left / right | In SEARCH or a NEW PLAYLIST name input, move the text cursor. From a list, right focuses NOW PLAYING; there, arrows select controls (left/right on the progress bar seek 10 seconds). On navigation tabs, select a tab |
| OK | Open a playlist/page, play a selected song, or activate the focused control/tab. On the SEARCH input or NEW PLAYLIST name, open the keyboard if hidden; **that press does not submit**. SEARCH matches retain normal selection |
| Long OK | Toggle the TV keyboard manually |
| Short Back | Hide an open keyboard first. Otherwise go back one view, return from the player to the list, or cancel a NEW PLAYLIST name |
| Long Back | Hide an open keyboard first. In SEARCH or a NEW PLAYLIST name, act as Back, not quit. Elsewhere, open quit confirmation |
| Short Menu | Open/resume SEARCH, focus its input and open the keyboard; from search detail pages, return to SEARCH. No action if the keyboard is already open. **Never types `/`** |
| Long Menu (keyboard hidden, outside sign-in) | Check for Android TV updates or reopen the available update |
| Play/pause (also play or pause) | Toggle playback outside text entry |
| Next / fast-forward | Next track outside text entry; not a seek command |
| Previous / rewind | Previous track outside text entry; not a seek command |
| Volume up / down | TV system volume |

Media keys are mapped to text keys in the terminal: while a SEARCH input
or NEW PLAYLIST name has focus, they can enter a space, `n` or `p` rather
than control playback. Hide the keyboard and focus a player control to
use media keys without editing text.

### Type a search or playlist name

1. Select the **SEARCH input** (Menu is a shortcut), or choose
   **+ NEW PLAYLIST** in the library to enter its name input.
2. Press **OK** to open the keyboard. Enter letters with the remote;
   edits reach nu11signal **live**, including keyboard composition.
3. Use the keyboard's submit key to open full search results or create the
   playlist. Or press **Back** to hide the keyboard without submitting:
   in SEARCH, move down to choose a match; on the name input, another OK
   reopens the keyboard.

Search matches start after **two characters** and wait until you stop
typing briefly before requesting results. Reopening the keyboard keeps
the existing term/name for appending. The keyboard hides when the input
loses focus or the typing mode ends.

### Quit

1. Hide the keyboard with **Back**, if open.
2. Leave SEARCH or cancel the NEW PLAYLIST name with Back, if needed.
3. **Hold Back** to open **QUIT NU11SIGNAL?**.
4. Press **OK** to confirm; short Back cancels. Confirming stops playback
   and closes the app. Home does **not** quit.

## Rain and the audio permission

At **first playback**, Android asks for the **Record audio** permission
(`RECORD_AUDIO`). It is not requested at launch.

- **Allow:** the rain can follow the TV's **output mix, not a microphone**.
  Other TV sounds can affect it too.
- **Decline:** music still plays; the rain stays decorative. The app does
  not ask again during that run; a permanent denial remains decorative.

To revoke or grant access later, open **Settings > Apps > nu11signal >
Permissions** and change the audio/microphone permission. Android's label
may mention a microphone even though this feature reads the output mix.
Unsupported devices fall back to decorative rain. Capture pauses while
music is paused, the app is in the background, or the app exits.

## Background playback

Press **Home** while playing: music continues from the launcher or while
using another app. Remote **play/pause**, **next** and **previous** still
work. The app publishes a now-playing notification/system media surface
with the title, artist and playback controls; its location depends on the TV.

- **Pause:** press play/pause from the launcher or use the media controls.
  After five minutes paused, the foreground notification is removed.
- **Stop completely:** return to nu11signal and [confirm quitting](#quit).
- **Return to browsing:** reopen nu11signal from the apps list.

Android can still end background playback under resource pressure. If that
happens, relaunch the app; the service cannot restart a killed player itself.

## Update or uninstall

### Update from the TV

1. When a newer TV APK is available, nu11signal offers it while the app is
   in front. To check manually or reopen a dismissed offer, hide the keyboard
   and **hold Menu** outside sign-in. Short Menu still opens search.
2. Choose **Download and install** (or **Later** to keep listening).
3. If prompted, choose **Open Settings**, allow installations from nu11signal,
   then press **Back**. Select the update again to continue. This permission
   does not allow silent installation.
4. Wait for download and validation. **Cancel** or Back cancels preparation;
   leaving the app prevents it from opening an installer in the background.
5. Android opens its own installer. Confirm **Install/Update** there, then
   reopen nu11signal when it finishes. Installation can interrupt playback.

Only stable releases from the official GitHub repository with a matching
`nu11signal-tv-<version>.apk` are offered. Desktop-only releases do not trigger
an update. Discovery checks at most 90 releases; a network error or exhausted
search limit is not reported as proof that the app is up to date. Automatic
checks are quiet on error; hold Menu to retry and see the result.

The app checks package identity, version and the official signing certificate
before handing a private read-only APK snapshot to Android. Android's installer
performs the final signature/integrity verification and requires your approval.
Updates keep app data and your Apple Music session; **do not uninstall first**.
Debug-signed builds cannot use this release-key update path.

If your TV has no compatible installer, does not allow this install source,
or a network/validation error persists, use the manual method below. After
several cancelled Android installer handoffs, fully exit and reopen nu11signal
before retrying; retained snapshots are bounded for safety.

### Manual update or removal

Download and check the newer release APK, then install it over the old one
using the installation steps above. Updates signed with the **same signing
key** keep app data and your Apple Music session.

**Uninstall:** open **Settings > Apps > nu11signal > Uninstall** and confirm.
This removes app data, including the sign-in session; reinstalling requires
signing in again.

## Troubleshooting

| Problem | What to do |
|---------|------------|
| Stuck on loading | Wake the TV, check its internet connection and wait up to **two minutes**. If still stuck, close and relaunch nu11signal |
| Sign-in finished but the terminal did not return | Press Back to leave the sign-in window/page, then relaunch. A completed session is kept; if the guide returns, finish Apple's sign-in prompts |
| No sound | Check the TV/output-device volume and mute state, then select a track and confirm it is playing. Record audio permission is **not** required for sound |
| Rain stays decorative or flat | Check **Settings > Apps > nu11signal > Permissions** for audio access and make sure a track is playing with the app in front. Paused music has no live levels; some devices cannot capture the output mix |
| Search seems slow | Type at least two characters, then pause typing: the search waits for you to stop. Check the network if matches still do not arrive |
| “App not installed” when updating | A different signing key (for example, a development APK vs a release) cannot update the old app. Prefer an APK signed with the original key; otherwise uninstall first, then install and **sign in again**. Uninstalling removes the old session |

For desktop controls and catalog behavior, see [Usage](usage.md). For building
an APK rather than installing a release, see [Releasing](releasing.md#android-tv-apk).
