---
name: web-research
description: Teaches when to fetch a web page with http_fetch. Use when the user asks about the content of a site on the operator's allow-list, or asks to check, read, or summarize a page.
license: Apache-2.0
---

# Web research

When the user asks what a page says, or asks you to check or summarize a
site, and the host is one the operator allow-listed:

1. Call the tool with the full URL: `TOOL: http_fetch(https://host/path)`
2. Read the OBSERVATION. If it is an error naming the allow-list, tell the
   user that host is not permitted — do not retry other hosts.
3. Answer from the fetched content only. Quote sparingly; summarize in your
   own words. If the page was truncated by the size cap, say so.

Never invent page content. If the tool was not executed (a shadow-mode
observation says so), tell the user the fetch was simulated and no real
page was read.
