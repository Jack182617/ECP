# Session Fixture

This disposable project models a session refresh rule. A refresh with an older
generation must never overwrite a newer authenticated session. The repository
is intentionally small so a fresh Codex task can inspect the whole contract.

Run the safe local check with:

```text
/usr/bin/python3 -m unittest discover -s tests
```
