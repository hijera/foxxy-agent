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
  • берёт последний semver-тег origin, бампит patch (X.Y.Z+1),
    но не ниже границы линии MIN_VERSION (сейчас 0.3.0)
  • пушит новый тег
  • через workflow_call запускает дочерние сборки:
      ├─ Release binaries   → CLI-бинарники + desktop .exe + SHA256SUMS → GitHub Release
      ├─ Docker build&push  → образ в ghcr.io/hijera/foxxy-agent
      ├─ IntelliJ plugin    → zip плагина → GitHub Release
      │                      + updatePlugins.xml → docs/ в main и ассет релиза (раздел 5)
      ├─ VS Code extension  → universal .vsix → GitHub Release
      └─ Release notes      → «What's Changed» в описание релиза — после Release binaries,
                              IntelliJ plugin и VS Code extension, даже если какая-то упала
```

Описание релиза пишет **отдельный job** (`.github/workflows/release-notes.yaml`), и запускается
он последним, а не внутри сборок. Три сборки прикладывают ассеты к одному релизу одновременно:
`softprops/action-gh-release` заводит в каждой свой черновик и оставляет самый ранний. Заметки,
сгенерированные внутри одной из сборок, пропадали, если первым успевал черновик другой, — так
вышли пустыми 0.2.87 и 0.2.91. Поэтому в первые минуты после merge у релиза уже есть ассеты, но
ещё нет описания — это нормально. Уже заполненное описание job не трогает.

Дополнительно `Release binaries` и `Docker build and push` независимо ловят
**ручной push тега** `X.Y.Z` (`on: push: tags`) и `workflow_dispatch`.

---

## 2. Разовая настройка репозитория (один раз)

1. **Права workflow-токена.** GitHub → **Settings → Actions → General →
   Workflow permissions** → выбрать **«Read and write permissions»** → **Save**.
   Без этого CI не сможет надёжно пушить тег и создавать Release
   (сейчас стоит `read`).

2. **Actions включены** — уже да (проверка: `gh api repos/hijera/foxxy-agent/actions/permissions`).

   **GitHub Pages** — тоже уже да: ветка `main`, папка `/docs`
   (проверка: `gh api repos/hijera/foxxy-agent/pages`). Именно это раздаёт
   `updatePlugins.xml` для IntelliJ (раздел 5), так что источник Pages менять нельзя.

3. **Линия версий.** Авто-бамп поднимает только последнее число, поэтому `0.2.95`
   сам по себе в `0.3.0` не превратится. Линию задаёт `MIN_VERSION` в шаге
   «Get latest SemVer tag and bump patch» файла `tag-on-merge.yaml` — нижняя граница
   версии: пока «старший тег + 1» меньше границы, релиз получает саму границу
   (`0.2.95` → `0.3.0`), дальше идут `0.3.1`, `0.3.2`… Чтобы начать следующую линию
   из [дорожной карты](../../ROADMAP.md), подними `MIN_VERSION` (и `releaseLineFloor` в
   `internal/update/tag_on_merge_workflow_test.go`) обычным PR — его мердж и выйдет
   как `0.4.0`. Тег руками для этого пушить не надо: ручной push тега собирает только
   бинарники и Docker-образ, без плагинов, `updatePlugins.xml` и описания релиза.
   Нумерация форка не связана с upstream-овской `0.9.x`.

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

Описание релизу, у которого его нет (ручной тег из способа B, релиз после
пересборки), дописывает **Actions → «Release notes» → Run workflow** с полем
`tag`. Существующее описание он не перезаписывает.

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
| `updatePlugins.xml` | документ репозитория плагинов (см. раздел 5) |

Плюс Docker-образ в `ghcr.io/hijera/foxxy-agent` (теги `X.Y.Z`, `X.Y`, `X`,
`latest`).

---

## 5. Репозиторий плагина для IntelliJ (автообновление)

JetBrains-репозиторий плагинов — это **один URL, отдающий `updatePlugins.xml`**. Сам zip
может лежать где угодно, поэтому отдельный сервер-зеркало не нужен: документ указывает
прямо на ассет GitHub Release, а роль «сервера обновлений» играет GitHub Pages этого
репозитория (ветка `main`, папка `docs/`).

Адрес, который пользователь один раз добавляет в
**Settings → Plugins → ⚙ → Manage Plugin Repositories → +**:

```text
https://hijera.github.io/foxxy-agent/updatePlugins.xml
```

Запасной адрес — тот же документ, приложенный к самому свежему релизу:

```text
https://github.com/hijera/foxxy-agent/releases/latest/download/updatePlugins.xml
```

После этого FoxxyCode ищется во вкладке **Marketplace** и обновляется штатным механизмом
IDE, как любой плагин из маркетплейса.

### Что делает пайплайн

На каждом релизном теге job `intellij-plugin.yaml` — уже **после** того, как zip приложен к релизу
(поэтому сбой этих шагов делает job красным, но не оставляет релиз без плагина):

1. читает `META-INF/plugin.xml` **из собранного архива** (а не из исходников) —
   `id`, `name`, `version`, `vendor`, `since-build`, `until-build`;
2. проверяет, что id — `dev.foxxycode.intellij`, имя — `FoxxyCode`, а версия внутри
   плагина совпадает с тегом; при расхождении job падает и документ не двигается;
3. прикладывает `updatePlugins.xml` к релизу — документ появляется только рядом с уже
   опубликованным zip и никогда не рекламирует ассет, которого ещё нет;
4. коммитит `docs/updatePlugins.xml` в `main` — это то, что раздаёт Pages. Пуш повторяется
   до трёх раз, если в `main` в этот момент влился соседний PR.

Документ всегда описывает **одну** версию: JetBrains разрешает указывать id плагина в
`updatePlugins.xml` только один раз. Старые версии никуда не деваются — они остаются
ассетами своих релизов, и любую можно поставить через **Install Plugin from Disk**.

Автоматически версия двигается только вперёд: пересборка старого тега
(`workflow_dispatch` в `tag-on-merge.yaml`) документ не откатит.

### Откат и ресинк вручную

**Actions → IntelliJ plugin repository → Run workflow**, поле `version` — тег `X.Y.Z`.
Плагин не пересобирается: берётся zip, уже приложенный к этому релизу. Это единственный
способ указать документу на **более старую** версию — например, когда свежий релиз
оказался плохим и всем нужно вернуть предыдущий.

### Если IDE не видит обновление

```bash
curl -fsS https://hijera.github.io/foxxy-agent/updatePlugins.xml
```

- **версия не сдвинулась** — посмотрите job `Publish updatePlugins.xml to GitHub Pages` в
  запуске `IntelliJ plugin`; Pages пересобирается ещё около минуты после коммита;
- **`plugin ID mismatch` / `plugin version mismatch` в логе** — в собранном архиве не тот
  `id` или версия не совпала с тегом; проверьте
  `editors/intellij/src/main/resources/META-INF/plugin.xml` и передачу `PLUGIN_VERSION` в
  Gradle;
- **версия сдвинулась, а IDE молчит** — проверьте, что ссылка `url=` из документа
  скачивается (`curl -I -L <url>`), и что `since-build` не выше сборки вашей IDE.

---

## 6. Диагностика

```bash
gh run list     -R hijera/foxxy-agent --limit 10   # запуски workflow
gh release list -R hijera/foxxy-agent              # созданные релизы
gh run view <run-id> -R hijera/foxxy-agent         # детали + логи джобов
```

- Гейт **«tag must be on main»**: если тег-коммит не является предком
  `origin/main`, сборка бинарников пропускается (в логе `skipping binary release`).
- **Релиз без «What's Changed»** (на странице только `Release X.Y.Z (auto-tag on PR merge)`).
  Сразу после merge так и должно быть, пока идут сборки. Если описание не появилось и после них,
  смотрите job `Release notes / Write the release notes` в запуске `Tag release on merge`, затем
  допишите описание вручную: **Actions → «Release notes»**, поле `tag`. Что сборки гонялись за
  создание релиза, видно в логе шага `Upload assets to GitHub Release` / `Attach … to GitHub Release`:
  `Using release … instead of duplicate draft …`. Это штатно, описание всё равно запишет `Release notes`.
- Предупреждения **«Node.js 20 is deprecated»** в логах безвредны.
- Если `gh` показывает не тот репозиторий — добавляй `-R hijera/foxxy-agent`
  (по умолчанию он может резолвить upstream `coddy-project/coddy-agent`).
