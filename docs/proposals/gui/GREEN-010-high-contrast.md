# GREEN-010: Honour the system's high contrast setting

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | claude/gui-5-release @ 7209952 |
| Area | gui/web/static/style.css |
| Effort | small |
| Decided by | (Grey) |

## Summary

When the system asks for more contrast (`prefers-contrast: more`), the
GUI uses darker secondary text, firmer lines and 2-pixel borders on
controls. This works in the light and dark themes and with the Settings
theme choice.

## Problem

Hints, labels and borders use light greys that are fine for most people
but faint for users with low vision, who often turn on the system's
"increase contrast" setting. The GUI ignored it.

## Idea

Override only the `--muted` and `--line` colour tokens and control border
widths inside a `prefers-contrast: more` media query. Every component
already uses those tokens, so nothing else changes.

## Cost and risk

About 10 lines of CSS, with no script. There is no automated check yet,
because the media feature cannot be emulated in the Playwright version CI
uses without extra set-up. It was checked by eye.

## Prototype

On the branch, in `style.css`.
