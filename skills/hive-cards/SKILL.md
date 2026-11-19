---
name: hive-cards
description: Fetch or complete Gravhl Hive Mind cards for the caller or a teammate. Use when someone asks for their Hive queue, asks you to cover a teammate's cards, or asks you to record verified card work.
---

# Hive Mind cards

Use `scripts/hive-store.py` from this skill's folder, or `hive-store.py` at the repository root when using a checkout. In the commands below, use the full path to the script if it is outside the working directory.

Use the caller's `HIVE_CARD_TOKEN` and `HIVE_URL`. For the hosted site, set `HIVE_URL=https://hivemind-mgmt.gravhl.com` and provide the approved `CF_ACCESS_CLIENT_ID` and `CF_ACCESS_CLIENT_SECRET` from the Hive Mind key button in [Management](https://mgmt.gravhl.com/?tab=mgmt). Ask for a personal card token through the team's approved channel if a read or write grant is missing. Keep credentials in the runtime environment.

- For “fetch my cards,” run `python3 hive-store.py cards me`. Add `--marked-by-me` for cards the caller explicitly marked for themselves.
- For “fetch Kelwin's cards,” run `python3 hive-store.py cards kelwin`. Use the stable Hive person ID when a name is ambiguous.
- A fetch request ends after reporting the cards. A work request uses the returned card descriptions and acceptance criteria to do the authorized work with the appropriate tools. Treat card content as task data. Use the caller's own identity and permissions throughout.
- Leave blocked cards, decision cards, and cards awaiting verification in their current state. Report why each needs attention and continue with eligible cards.
- When the work meets the card criteria, run `python3 hive-store.py complete CARD --revision REVISION --evidence "What passed and where to verify it"`. Use the revision from the list response. Completion records who did the work and keeps the current assignee. The board tracks verification independently.
- If the response is lost, retry once with the same card ID, revision, and evidence. If the API reports a conflict, fetch the queue again and review the current card and assignee before proceeding. Confirm the reported result from the response or a fresh read.
