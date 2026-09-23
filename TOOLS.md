# Tools

Every tool returns plain text. Failures come back as an MCP tool error (`isError: true`), and the text says what went wrong.

## Shared types

```ts
/** Options accepted by every tool that acts on a page. */
interface Output {
  /** Seconds to wait after the action before the aria dump / screenshot is taken. Default 0.5, max 30. */
  wait?: number;
  /**
   * false: no aria dump.
   * true: the full aria dump.
   * string: only the lines that contain this text (case insensitive), with their subtree and ancestors.
   *         An element id like "e12" returns only that element and its subtree.
   */
  aria: boolean | string;
  /** Take a PNG screenshot of the viewport. The result contains a url, the image is deleted after 5 minutes. */
  screenshot: boolean;
}

interface SessionArgs {
  /** The session id returned by start_session. */
  session_id: string;
}

type Resolution =
  | "desktop"        // 1440x900 (default)
  | "iphone"         // 390x844
  | "ipad_landscape"; // 1180x820
```

## Result format

The page tools return text like this:

```
session: 3f9a1c2b7d4e
result: clicked e6
events since last call:
  - alert dialog "Saved!" was accepted
  - console error: Failed to load resource
url: http://localhost:3000/login
title: Login
screenshot: http://localhost:8080/screenshots/13d9….png (deleted after 5m0s)
viewport: desktop 1440x900, scrolled to 0,0 of 1440x2400
aria:
main
  heading[1] "Sign in"
  [e5] textbox "Email" value="me@example.com" required focused
  [e6] button "Sign in"
```

- `events` only appears when something happened. Events are dialogs (always accepted), page errors, `console.error` output and closed popups.
- `screenshot` only appears with `screenshot: true`.
- `viewport` and `aria` only appear when `aria` is not `false`.
- When an action fails and `aria` or `screenshot` was requested, the error text is followed by this same report, so you can see what the page looks like.

### Aria dump syntax

| syntax | meaning |
| --- | --- |
| indentation | nesting |
| `[e12]` | id of an interactive element, use it with the `*_by_id` tools |
| `heading[2]` | heading level |
| `"…"` directly after the role | accessible name |
| `"…"` alone on a line | text content |
| `value="…"`, `placeholder="…"` | current value / placeholder of an input (passwords are masked) |
| `-> /path` | link target (same origin links are shown as a path) |
| `testid=…` | `data-testid` / `data-test` / `data-cy` attribute |
| `row: a \| b \| c` | a table row of plain cells |
| trailing words | states: `checked`, `mixed`, `disabled`, `expanded`, `collapsed`, `selected`, `pressed`, `current`, `required`, `readonly`, `invalid`, `open`, `focused` |

`clickable` is an element without a role that looks clickable (pointer cursor, `onclick` or `tabindex`). Ids stay stable while the document lives, and they are never reused within a session.

## Session tools

### `start_session`

Starts a session with a single headless Firefox tab and opens `url`.

- The tab may only visit `allowed_domains`. Navigating anywhere else closes the session.
- A session is closed after 5 minutes without a tool call.
- New tabs and popups are closed and reported as events.

```ts
function start_session(args: Output & {
  /** Absolute http(s) url to open. */
  url: string;
  /**
   * Domains the tab may visit, each one also allows its subdomains. Defaults to the domain of url.
   * When the server has ALLOWED_DOMAINS set, every domain here must be (a subdomain of) one of those.
   */
  allowed_domains?: string[];
  /** Default "desktop". */
  resolution?: Resolution;
}): string;
```

### `stop_session`

Closes the session and its tab.

```ts
function stop_session(args: SessionArgs): string;
```

### `keep_alive`

Resets the inactivity timer of the session.

```ts
function keep_alive(args: SessionArgs): string;
```

### `session_count`

Returns the number of open sessions on the server as a plain number, e.g. `3`. It takes no arguments, and its calls are not logged.

```ts
function session_count(args: {}): string;
```

## Page tools

All page tools run one at a time per session. Pointer and keyboard input uses real browser input, so the page sees `isTrusted: true` events.

### `get_page`

Returns the page state without doing anything.

```ts
function get_page(args: SessionArgs & Output): string;
```

### `click_by_id`

Scrolls the element into view and clicks its center.

```ts
function click_by_id(args: SessionArgs & Output & {
  /** Element id from the aria dump, e.g. "e12". */
  id: string;
}): string;
```

### `click_by_text`

Clicks a visible element found by its text. If nothing matches the text, it tries the aria-label and then the title. An exact match is tried first, then a case insensitive substring match.

```ts
function click_by_text(args: SessionArgs & Output & {
  /** Visible text, aria-label or title of the element. */
  text: string;
  /** Only accept an exact, case sensitive match. Default false. */
  exact?: boolean;
  /** Zero based index when multiple elements match. Default 0. */
  nth?: number;
}): string;
```

### `type_by_id`

Clicks the element to focus it, then types.

```ts
function type_by_id(args: SessionArgs & Output & {
  /** Element id from the aria dump, e.g. "e12". */
  id: string;
  /** Text to type. */
  text: string;
  /** Select all and delete the current value first. Default false. */
  clear?: boolean;
  /** Press Enter after typing. Default false. */
  submit?: boolean;
}): string;
```

### `type_by_text`

Finds an input by its label, then its placeholder, then its text. It clicks the input to focus it, then types.

```ts
function type_by_text(args: SessionArgs & Output & {
  /** Label, placeholder or text of the input. */
  target: string;
  /** Text to type. */
  text: string;
  /** Select all and delete the current value first. Default false. */
  clear?: boolean;
  /** Press Enter after typing. Default false. */
  submit?: boolean;
  /** Only accept an exact, case sensitive match of target. Default false. */
  exact?: boolean;
  /** Zero based index when multiple elements match. Default 0. */
  nth?: number;
}): string;
```

### `press_key`

Presses keys or shortcuts in order. They go to whatever element has focus.

```ts
function press_key(args: SessionArgs & Output & {
  /**
   * At least one key. Key names follow KeyboardEvent.key: "Enter", "Escape", "Tab", "ArrowDown", "PageDown", "a", …
   * Combine with modifiers using "+": Shift, Control, Alt, Meta, ControlOrMeta.
   * Examples: ["Escape"], ["Control+k"], ["Tab", "Tab", "Enter"]
   */
  keys: string[];
}): string;
```

### `scroll`

Moves the mouse to `x,y` and scrolls with the mouse wheel. The position decides which scroll container scrolls.

```ts
function scroll(args: SessionArgs & Output & {
  /** X position in the viewport, in css pixels. */
  x: number;
  /** Y position in the viewport, in css pixels. */
  y: number;
  /** Pixels to scroll horizontally, positive scrolls right. Default 0. */
  delta_x?: number;
  /** Pixels to scroll vertically, positive scrolls down. Default 0. */
  delta_y?: number;
}): string;
```

### `set_resolution`

Changes the viewport size. The user agent stays the same.

```ts
function set_resolution(args: SessionArgs & Output & {
  resolution: Resolution;
}): string;
```

### `navigate`

Opens a url in the session tab. The url must be within the session's allowed domains.

```ts
function navigate(args: SessionArgs & Output & {
  url: string;
}): string;
```

### `select_option`

Selects an option of a native `<select>`. The options are listed in the aria dump under the combobox or listbox.

```ts
function select_option(args: SessionArgs & Output & {
  /** Id of the select element from the aria dump. */
  id: string;
  /** Label or value of the option. */
  option: string;
}): string;
```
