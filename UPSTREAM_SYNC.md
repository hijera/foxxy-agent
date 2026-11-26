# Upstream sync tracker

Operational index for manual ports from `https://github.com/coddy-project/coddy-agent`
(remote `upstream`) into FoxxyCode. A direct `git merge upstream/main` is not used.

- **Per-commit decisions:** `ports.yaml` is the authoritative ledger.
- **Current state and standing divergences:** this file.
- **Detailed wave reports, validation logs and old follow-ups:**
  `docs/plans/upstream-sync-history.md`; read them on demand, not for every sync.

## Current state

| Field | Value |
| --- | --- |
| Last full sync | Upstream `1.2.79`, 2026-11-25 |
| Upstream checkpoint | `cd42d7df06fb3c64c1b482e7c4463f3fc239ebb4` |
| Previous checkpoint | `d81d6cca0c12716035a0539130cab8ac66fe2a27` (`1.2.73`) |
| Fork implementation | `7cdd8efa3b01994db27b9cb39d1504e8a74542a5` |
| Ledger backlog | No `partial` or `deferred` entries as of 2026-11-26 |
| Wave report | `docs/plans/upstream-sync-history.md`, section “Sync through upstream 1.2.79” |

Selective and out-of-order ports do **not** move the full-sync checkpoint. An upstream
revision named as the comparison base of such a port is not itself a synchronization
checkpoint.

## Selective ports after the last full sync

### Mode restoration when switching sessions — 2026-11-24

Sources: upstream `2dda4dbf12e215cebfcab23511e536250ecc78ce` (`fix(ui): keep mode and model per session`) and the shared settings-read part of `3c495d7a126c4757da6ba71bc76a1fd450c99637` (`fix(ui): settle the opened session's settings by the server's answer, not by History`). Checked against `upstream/main` at `aa36eb3fbaa53546461c6bb5348565b34ff7652b`; this is not a full sync through that revision.

- Ported the settings-only read used when a running session's transcript is restored from its stream shadow, and the shared settings mirror used by transcript and settings-only reads. Settings now follow the viewed session independently of the transcript paint fence.
- Kept FoxxyCode's top-level `mode` snapshot and once-per-open selection guards. Added the fork-specific reset of those guards for Back/Forward hash navigation, which does not pass through `pickSession`.
- Regression coverage in `external/ui/src/ui/App.sessionSelection.test.tsx`: a hand-picked Ask mode survives leaving and reopening its session, and two simultaneously running Plan/Ask sessions restore their own mode on every switch without waiting for a permission or turn event.

These source commits are not yet recorded in `ports.yaml`; record them once their fork
implementation commits are available in the receiving branch's history.

## Next sync

Resolve a fixed target after fetching, then use the current full-sync checkpoint:

```bash
git fetch upstream --prune
base=cd42d7df06fb3c64c1b482e7c4463f3fc239ebb4
target=$(git rev-parse upstream/main)
python scripts/upstream.py scan --from "$base" --to "$target"
python scripts/upstream.py wave --from "$base" --to "$target"
```

1. Cross-check `unreviewed` candidates against the selective ports above and the fork
   history. Absence from `ports.yaml` means unreviewed, not necessarily unported.
2. Follow `.claude/skills/upstream-port/SKILL.md`. For every affected file, inspect
   `fork(...)` markers and the matching registry entry before taking an upstream hunk.
3. Run the relevant behavioral guards and the required project gates. Record every
   reviewed upstream commit in `ports.yaml`, including skips and partial ports.
4. Move the checkpoint above only when the whole selected range is accounted for and
   no unresolved part remains. Add later out-of-order ports to the selective list.
5. Keep this file operational. Put long narratives, test logs and superseded reports in
   `docs/plans/upstream-sync-history.md` instead of growing this file with each wave.

## Permanent divergence registry

The English source of truth, including code sites and guard tests, is
`.claude/rules/upstream-divergences.md` (mirrored as
`.cursor/rules/upstream-divergences.mdc`). Every code site has a `// fork(<id>)`
marker. `internal/forkguard` checks the markers, guard tests and the IDs in both
registry copies.

Before taking an upstream change, run `git grep -n 'fork(' -- <file>` and follow the
“Next upstream change” instruction. A new owner decision needs a row in both registries,
code markers and a guard test in the same change. Older per-wave decisions are archived
in `docs/plans/upstream-sync-history.md`; promote one here when a later port touches it.

<!-- divergence-registry:start -->
| ID | Upstream | Форк | При следующем изменении upstream |
|----|----------|------|----------------------------------|
| `queue-output-limit` | 1.2.27 starts queued follow-ups only after end_turn | After max_tokens, deliver unread steer in order before after_turn; keep errors and Stop waiting | Preserve the owner-selected output-cap exception, steering priority and queue bounds |
| `retry-max-default` | 1.2.0 (#329): 3 shared extra attempts | Owner-selected default of 10; about 55m30s for a wholly silent model with the default guards and delays | Preserve the shared default and explicit 0 disabling retries |
| `stall-guard-layer` | 1.1.47: `stallGuardTransport` в `transport.go` режет SSE-тело без байтов, внутри ретрай-обёртки | Guard оборачивает провайдер и считает чанки (включая `Progress`), не keep-alive; стоит снаружи ретраев | 1.1.52: режимы портированы без `stallGuardTransport`/`idleBody`; при следующих изменениях сохранять слой guard |
| `transport-debug-wrap` | 1.1.47: общий транспорт на настройку прокси из `http.DefaultTransport`, пинги HTTP/2 через x/net | Тот же общий пул, но из форковых прокси-хелперов (таймаут заголовков, системный прокси #124, пинги `http.HTTP2Config` из #125) и за debug-обёрткой | 1.1.52: режимы портированы в `buildLLMTransport` с системным прокси, debug-обёрткой, ключом пула и форковым обходом NO_PROXY/loopback; при следующих изменениях сохранять их |
| `continue-path` | 1.1.47: stall после текста завершает ход; 1.2.9: recovery после 5xx/обрыва, пауза от `llm_retry_base_ms`, выключается `llm_retry_max: 0`, уведомление в UI-лог | Оборванный ответ продолжается под `agent.llm_continue`; паузы `llm_continue_stall_delays_ms` и `llm_continue_error_delays_ms`, Retry-After до `llm_continue_retry_after_max_ms`; парковка через `llm_retry`; оборванный текст на обоих путях обрезается по границе возобновления (`trimToResumeBoundary`): завершённая последняя строка остаётся целиком, оборванная — до последнего конца строки (1.2.9 хранит целиком); сброс Winsock (10053/10054) — временный сбой | Остаток 1.2.9: ветку форка сохранить, изменения `IsTransientProviderError` брать |
| `cut-keeps-complete-calls` | Оборванный ответ сохраняет текст и рассуждения, но tool call'ы незавершённого ответа не выполняются: ридеры их отбрасывают, сторож обрыва обнуляет, цикл сохраняет частичный ответ без них | Вызов, чьи аргументы пришли целым JSON, — шаг, который модель дозапросила, он выполняется рядом с обрывом и ход продолжается по его результатам; вызов с оборванным JSON по-прежнему отбрасывается. Не действует на Stop пользователя и на срез деградирующего ответа сторожем цикла | Сохранять сторону форка: целые вызовы выполняются |
| `continue-budget` | 1.2.35: five consecutive background recoveries, each spending a step | Per-turn continuation budgets: interactive 3; delegated and scheduled 5 with 5/20/80/120/120-second delays; memory retains the interactive budget. No max_turns spending. | Keep the shared transport/no-answer retry allowance separate; llm_continue controls both continuation policies. |
| `stall-timeout-alias` | Ключ `llm_stream_idle_timeout_ms` | То же имя; `llm_stall_timeout_ms` читается как устаревший алиас | Держать алиас, пока владелец его не уберёт |
| `stop-notice-transcript` | 1.2.9: `StopNotice` в UI-лог, отдельно печатается в Telegram, консоли и `-p` | Уведомление стримится в ответ и пишется в транскрипт; `StopNotice` — только в `meta.stop_notice`, remote-клиент и отчёт субагента | При порте 1.2.9 не брать печать `StopNotice` в `bot.go`, `print.go`, `app.go` |
| `session-disk-refresh` | Нет: процесс отвечает по своей копии сессии | Ход перечитывает сессию, изменённую другим процессом; устаревшая копия не перезаписывает историю | Если upstream сделает свою синхронизацию — свести с владельцем |
| `progress-chunks` | До 1.1.67 кадр без содержимого ничего не сообщает; 1.1.68 добавляет свой progress аргументов | `Progress` на каждый кадр без содержимого, мимо `emit` | 1.1.68: слить с форковым, правило «progress не ставит `emitted`» сохранить |
| `switch-enable-alias` | Переключатели — `enable`, `enabled` неизвестен | `enable` канонический, `enabled` — алиас | Ничего: алиас покрывает новые переключатели |
| `prompt-file-encoding` | 1.2.1 port | Owner-approved fork behavior; see the English registry and guard tests | Preserve on later ports |
| `retry-recovery-steps` | 1.2.1 port | Owner-approved fork behavior; see the English registry and guard tests | Preserve on later ports |
| `compaction-model-override` | 1.1.64 (#312) | The per-call summarizer works with Coddy after OpenCode was retired | Preserve the override and fallback chain on later ports |
| `compaction-summary-gate` | 1.2.72 port | Гейт `start_percent` для проекции суммаризатора поднят: эвакуация результатов включена всегда, протухшее чтение не попадает в сводку как актуальное; живые запросы LLM гейт сохраняют | Сохранять поднятие гейта при следующих портах compaction |
| `composer-send-mode` | 1.2.1 port | Owner-approved fork behavior; see the English registry and guard tests | Preserve on later ports |
| `session-asset-files` | 1.2.1 port | Owner-approved fork behavior; see the English registry and guard tests | Preserve on later ports |
| `background-server-lifetime` | 1.2.1 port | Owner-approved fork behavior; see the English registry and guard tests | Preserve on later ports |
| `cumulative-turn-usage` | Обновления usage несут цифры по вызовам, потребители их суммируют | Обновления token_usage несут кумулятивные счётчики хода; каждый потребитель берёт последнее обновление вместо суммы снимков: HTTP-коллекторы, строка задачи субагента и футер консоли | Сохранять правило «последнее кумулятивное побеждает»; формат на проводе менять только с решением владельца |
| `loopback-sign-in` | Исключений у формы входа нет, она показана всем | `foxxycode http` (его запускают плагины редакторов и desktop-приложение) пропускает прямого loopback-клиента мимо формы входа, пока не настроен bearer-токен; страница браузера на другом loopback-порту тоже считается прямым loopback-клиентом (решение владельца 2026-11-26: доступом управляют CORS-допуск и предупреждение при запуске, вторых ворот нет); `foxxycode serve` исключений не делает | Сохранять исключение и никогда не сужать его до клиентов того же origin |
<!-- divergence-registry:end -->

## Fork origin

FoxxyCode shares Git history with coddy-agent at `9370936` (`0.9.26-4-g9370936`). The
first fork commit is `b74dc21`; the first fork tag containing the shared base is `0.2.2`.
Fork and upstream release numbers are independent.

```bash
git merge-base HEAD upstream/main
git describe --tags "$(git merge-base HEAD upstream/main)"
```

Everything after the shared base arrives through the waves recorded in `ports.yaml` and
the archived journal; fork commits do not flow back to upstream.
