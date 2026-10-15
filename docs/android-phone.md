# Android phones and tablets

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

The Android TV APK also runs on Android phones and tablets. The same app
adapts to the device: on a phone or tablet the text keeps a readable size,
taps and swipes replace the remote, and the phone keyboard opens for search.
This page covers what differs from the [Android TV guide](android-tv.md);
everything else there applies as written.

## Requirements

- An Android phone or tablet running **Android 9 or later** (API 28+).
  Tested on a **Samsung Galaxy Z Fold7**, folded and unfolded.
- An **Apple Music subscription** and your Apple ID sign-in details.

## Install

1. On the phone, open
   [GitHub Releases](https://github.com/wahh-22/nu11signal/releases) and
   download `nu11signal-tv-<version>.apk` from a release with Android assets.
   To check it first, download the `.sha256` file too and follow the checksum
   steps in the [Android TV guide](android-tv.md#download-and-check-on-a-computer)
   on a computer.
2. Open the downloaded APK. If Android asks, allow your browser or file
   manager to **install unknown apps**, then go back and confirm **Install**.
3. Open **nu11signal** from your apps.

The app keeps the TV package name, so a phone and a TV use the same APK and
the same updates.

## Sign in

The first launch opens Apple Music's page. Tap **Sign In** at the top right,
then enter your Apple ID, password and verification code with the phone
keyboard. A hint at the bottom of the screen reminds you that **Back**
returns to nu11signal. When you finish, nu11signal returns by itself and
keeps the session.

## Use

| Action | How |
|--------|-----|
| Open a playlist, press a button or a tab | Tap it |
| Scroll a list | Swipe up or down over it |
| Search | Tap **SEARCH**; the keyboard opens, type, then press the keyboard's send key |
| Go back | Android's Back gesture or button |
| Turn the screen | The layout follows: side by side in landscape, NOW PLAYING over the list in portrait |

Every on-screen control accepts a tap, so the keyboard shortcuts in the hint
line are optional on a phone.

## Background playback, rain and updates

These work as on a TV; see the [Android TV guide](android-tv.md):

- [Background playback](android-tv.md#background-playback): leave the app
  while music plays; the notification carries the playback controls.
- [Rain and the audio permission](android-tv.md#rain-and-the-audio-permission):
  allow **Record audio** at first playback for rain that follows the music.
- [Updates](android-tv.md#update-from-the-tv): a newer release is offered
  when the app comes to the front. Phones have no Menu key, so there is no
  manual check: to look for an update again, quit nu11signal (or swipe it
  away from recent apps) and open it again.

## Known limits

- Screenshots of the app can come out black on some phones while Apple Music's
  protected playback is active.
- Local music files are not supported, as on the TV.
