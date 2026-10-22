### Fanfare

- The finale's Stop dialog shows a picture left of the text: `cover.png`, `cover.jpg` or `cover.icns` from the played pair's folder, else mw's built-in party-popper icon (written once to `<dir>/.mw-fanfare-icon.png`). macOS and zenity 3.90+ only; older zenity and kdialog show none, and Windows shows the information icon.
- Test fix: `TestLogSinceStartAndResume` compared RFC3339Nano timestamps as strings (trailing zeros are dropped, so string order is not time order) and failed intermittently; it now parses them.
