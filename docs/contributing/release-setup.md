# Настройка релизов в GitHub

Как выпускать сборки foxxyCode на `github.com/hijera/foxxy-agent` — тот же набор
дистрибутивов, что и у upstream coddy, плюс foxxy-специфичный desktop `.exe`.

> **Почему сейчас релизов нет.** На форке все workflow'ы активны, но их ни разу
> не запускали: коммиты идут напрямую в `main` (без PR), а версионные теги
> `X.Y.Z` в `origin` не пушились. Оркестратор `Tag release on merge` срабатывает
> только на **смердженный PR** в `main` (или ручной запуск), а `Release binaries`
> и `Docker` дополнительно ловят **push тега** вида `X.Y.Z`. Ни того, ни другого
> на форке не происходило — отсюда 0 релизов.
>
> Релизы `coddy_0.9.35_*`, которые видны через `gh` без флага `-R`, принадлежат
> **upstream `coddy-project/coddy-agent`**, а не форку. Всегда указывай репозиторий:
> `gh release list -R hijera/foxxy-agent`.

---

## 1. Как устроен релиз-пайплайн

```
merge PR в main
      │
      ▼
Tag release on merge (.github/workflows/tag-on-merge.yaml)
  • берёт последний semver-тег origin, бампит patch (X.Y.Z+1)
  • пушит новый тег
  • через workflow_call запускает дочерние сборки:
      ├─ Release binaries   → CLI-бинарники + desktop .exe + SHA256SUMS → GitHub Release
      ├─ Docker build&push  → образ в ghcr.io/hijera/foxxy-agent
      ├─ IntelliJ plugin    → zip плагина → GitHub Release
      └─ VS Code extension  → universal .vsix → GitHub Release
```

Дополнительно `Release binaries` и `Docker build and push` независимо ловят
**ручной push тега** `X.Y.Z` (`on: push: tags`) и `workflow_dispatch`.

---

## 2. Разовая настройка репозитория (один раз)

1. **Права workflow-токена.** GitHub → **Settings → Actions → General →
   Workflow permissions** → выбрать **«Read and write permissions»** → **Save**.
   Без этого CI не сможет надёжно пушить тег и создавать Release
   (сейчас стоит `read`).

2. **Actions включены** — уже да (проверка: `gh api repos/hijera/foxxy-agent/actions/permissions`).

3. **(Опционально) продолжить нумерацию `0.9.x`.** Авто-бамп смотрит на теги
   **origin**, а там их нет → базой станет `0.1.0` и первый авто-релиз будет
   `0.1.1`. Чтобы продолжить линию coddy, один раз запушь базовый тег в origin:

   ```bash
   git push origin 0.9.35     # или актуальную версию
   ```

---

## 3. Как выпустить релиз — три способа

### A. Через PR (штатный путь)

```bash
git switch -c my-change
# ...правки, коммиты...
git push -u origin my-change
```
Открыть PR в `main` на GitHub → **Merge**. `Tag release on merge` сам создаст
тег и релиз со всеми артефактами.

> ⚠️ Прямой `git push` в `main` релиз **не** создаёт — нужен именно merge PR
> либо способы B/C.

### B. Ручной тег (быстрее всего получить релиз прямо сейчас)

Коммит уже должен быть в `origin/main` (гейт «tag must be on main»):

```bash
git push origin main            # если ещё не запушен
git tag 0.9.36                  # следующая версия
git push origin 0.9.36
```
Запустятся `Release binaries` + `Docker build and push` по `push: tags`.

### C. Вручную из UI (workflow_dispatch)

GitHub → **Actions → «Tag release on merge» → Run workflow**:
- пустой ввод — бампит тег на текущем `HEAD` ветки `main` и делает релиз;
- `release_tag = X.Y.Z` — пересобирает уже существующий тег без бампа.

Также можно запустить отдельно **«Release binaries»** с полем `tag`.

---

## 4. Что появится в результате

GitHub Release (`hijera/foxxy-agent/releases`) с ассетами:

| Ассет | Что это |
|---|---|
| `foxxycode_<ver>_linux_amd64.tar.gz` / `_linux_arm64.tar.gz` | CLI Linux |
| `foxxycode_<ver>_darwin_amd64.tar.gz` / `_darwin_arm64.tar.gz` | CLI macOS |
| `foxxycode_<ver>_windows_amd64.zip` | CLI Windows |
| `foxxycode-desktop_<ver>_windows_amd64.zip` | **Desktop-приложение (WebView2)** |
| `SHA256SUMS` | контрольные суммы всех архивов |
| `*.vsix` | расширение VS Code (universal) |
| `*.zip` (IntelliJ) | плагин JetBrains |

Плюс Docker-образ в `ghcr.io/hijera/foxxy-agent` (теги `X.Y.Z`, `X.Y`, `X`,
`latest`).

---

## 5. Диагностика

```bash
gh run list     -R hijera/foxxy-agent --limit 10   # запуски workflow
gh release list -R hijera/foxxy-agent              # созданные релизы
gh run view <run-id> -R hijera/foxxy-agent         # детали + логи джобов
```

- Гейт **«tag must be on main»**: если тег-коммит не является предком
  `origin/main`, сборка бинарников пропускается (в логе `skipping binary release`).
- Предупреждения **«Node.js 20 is deprecated»** в логах безвредны.
- Если `gh` показывает не тот репозиторий — добавляй `-R hijera/foxxy-agent`
  (по умолчанию он может резолвить upstream `coddy-project/coddy-agent`).
