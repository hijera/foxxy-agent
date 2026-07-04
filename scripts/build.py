#!/usr/bin/env python3
"""Interactive and non-interactive build wizard for FoxxyCode Agent."""

from __future__ import annotations

import argparse
import hashlib
import os
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import zipfile
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable, Iterable, List, Optional, Sequence, Tuple

REPO_ROOT = Path(__file__).resolve().parents[1]

VERSION_PKG = "github.com/hijera/foxxycode-agent/internal/version.Version"

ALL_TAGS = ("http", "ui", "scheduler", "memory", "gateway.telegram", "gateway")

PRESETS: dict[str, list[str]] = {
    "lean": [],
    "full": ["http", "ui", "scheduler", "memory"],
    "gateway": ["http", "ui", "scheduler", "memory", "gateway.telegram"],
}

RELEASE_TARGETS: list[tuple[str, str]] = [
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("windows", "amd64"),
]

GO_TO_VSCE: dict[tuple[str, str], str] = {
    ("linux", "amd64"): "linux-x64",
    ("linux", "arm64"): "linux-arm64",
    ("darwin", "amd64"): "darwin-x64",
    ("darwin", "arm64"): "darwin-arm64",
    ("windows", "amd64"): "win32-x64",
}


@dataclass
class BuildOptions:
    target: str = ""
    tags: list[str] = field(default_factory=list)
    preset: str = ""
    goos: str = ""
    goarch: str = ""
    all_release: bool = False
    archive: bool = False
    ldflags_strip: bool = False
    plugin_version: str = ""
    production: bool = True
    vscode_targets: list[str] = field(default_factory=list)
    dry_run: bool = False
    no_color: bool = False


class UI:
    """Console output with optional ANSI colors."""

    def __init__(self, use_color: bool) -> None:
        self.use_color = use_color

    def _c(self, code: str, text: str) -> str:
        if not self.use_color:
            return text
        return f"\033[{code}m{text}\033[0m"

    def info(self, msg: str) -> None:
        print(self._c("36", f"[i] {msg}"))

    def ok(self, msg: str) -> None:
        print(self._c("32", f"[+] {msg}"))

    def warn(self, msg: str) -> None:
        print(self._c("33", f"[!] {msg}"))

    def err(self, msg: str) -> None:
        print(self._c("31", f"[x] {msg}"), file=sys.stderr)


def configure_stdio() -> None:
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            try:
                stream.reconfigure(encoding="utf-8")
            except (AttributeError, OSError, ValueError):
                pass


def host_platform() -> tuple[str, str]:
    system = platform.system().lower()
    machine = platform.machine().lower()
    goos = {"windows": "windows", "darwin": "darwin", "linux": "linux"}.get(system, "linux")
    goarch = {"amd64": "amd64", "x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(
        machine, machine
    )
    return goos, goarch


def bin_name(goos: str) -> str:
    return "foxxycode.exe" if goos == "windows" else "foxxycode"


def which(cmd: str) -> Optional[str]:
    return shutil.which(cmd)


def run_cmd(
    ui: UI,
    args: Sequence[str],
    *,
    cwd: Optional[Path] = None,
    env: Optional[dict[str, str]] = None,
    dry_run: bool = False,
    shell: bool = False,
) -> None:
    display = " ".join(args) if not shell else str(args[0])
    ui.info(f"Выполнение: {display}")
    if dry_run:
        return
    merged = os.environ.copy()
    if env:
        merged.update(env)
    subprocess.run(
        list(args) if not shell else args,
        cwd=str(cwd or REPO_ROOT),
        env=merged,
        check=True,
        shell=shell,
    )


def require_tools(ui: UI, names: Iterable[str]) -> None:
    hints = {
        "go": "Установите Go 1.25+ (см. go.mod) и добавьте в PATH.",
        "git": "Установите Git и добавьте в PATH (нужен для версии сборки).",
        "npm": "Установите Node.js с npm (нужен для UI и плагинов).",
        "node": "Установите Node.js и добавьте в PATH.",
        "java": "Установите JDK 17+ и добавьте java в PATH.",
        "make": "Установите GNU Make (для ui-build на Unix) или используйте прямой вызов npm.",
    }
    for name in names:
        if which(name) is None:
            ui.err(f"Не найден инструмент: {name}. {hints.get(name, '')}")
            raise SystemExit(1)


def git_version() -> str:
    def git(*a: str) -> Optional[str]:
        try:
            r = subprocess.run(
                ["git", *a],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                check=False,
            )
            if r.returncode == 0:
                return r.stdout.strip() or None
        except OSError:
            pass
        return None

    point = git("tag", "-l", "--points-at", "HEAD", "--sort=-v:refname")
    if point:
        return point.splitlines()[0]
    desc = git("describe", "--tags", "--dirty")
    if desc:
        return desc
    desc = git("describe", "--tags", "--always", "--dirty")
    if desc:
        return desc
    return "dev"


def normalize_tags(tags: Sequence[str]) -> list[str]:
    out: list[str] = []
    for t in tags:
        t = t.strip()
        if not t:
            continue
        if t not in out:
            out.append(t)
    if "gateway" in out and "gateway.telegram" not in out:
        out.append("gateway.telegram")
    if "ui" in out and "http" not in out:
        raise ValueError("Тег ui требует http — встраиваемый SPA доступен только с HTTP-шлюзом.")
    return out


def tags_from_preset(preset: str) -> list[str]:
    if preset not in PRESETS:
        raise ValueError(f"Неизвестный пресет: {preset}")
    return list(PRESETS[preset])


def tags_need_ui_build(tags: Sequence[str]) -> bool:
    return "http" in tags and "ui" in tags


def npm_cmd() -> str:
    return "npm.cmd" if platform.system() == "Windows" else "npm"


def ui_build(ui: UI, dry_run: bool) -> None:
    require_tools(ui, ["npm"])
    npm = npm_cmd()
    ui_dir = REPO_ROOT / "external" / "ui"
    run_cmd(
        ui,
        [npm, "install", "--no-fund", "--no-audit"],
        cwd=ui_dir,
        dry_run=dry_run,
    )
    run_cmd(
        ui,
        [npm, "run", "build:go"],
        cwd=ui_dir,
        dry_run=dry_run,
    )


def ldflags(version: str, strip: bool) -> str:
    parts: list[str] = []
    if strip:
        parts.extend(["-s", "-w"])
    parts.append(f"-X {VERSION_PKG}={version}")
    return " ".join(parts)


def go_build_one(
    ui: UI,
    *,
    goos: str,
    goarch: str,
    tags: Sequence[str],
    version: str,
    out_path: Path,
    strip: bool,
    dry_run: bool,
) -> Path:
    require_tools(ui, ["go"])
    out_path.parent.mkdir(parents=True, exist_ok=True)
    tag_str = ",".join(tags) if tags else ""
    args = ["go", "build"]
    if tag_str:
        args.extend(["-tags", tag_str])
    args.extend(
        [
            "-trimpath",
            "-ldflags",
            ldflags(version, strip),
            "-o",
            str(out_path),
            "./cmd/foxxycode/",
        ]
    )
    env = {"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"}
    run_cmd(ui, args, env=env, dry_run=dry_run)
    return out_path


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def pack_archive(ui: UI, binary: Path, archive_path: Path, goos: str, dry_run: bool) -> None:
    if dry_run:
        ui.info(f"Упаковка (dry-run): {archive_path}")
        return
    archive_path.parent.mkdir(parents=True, exist_ok=True)
    name = binary.name
    if archive_path.suffix == ".zip":
        with zipfile.ZipFile(archive_path, "w", zipfile.ZIP_DEFLATED) as zf:
            zf.write(binary, arcname=name)
    else:
        with tarfile.open(archive_path, "w:gz") as tf:
            tf.add(binary, arcname=name)
    ui.ok(f"Архив: {archive_path}")


def write_sha256sums(ui: UI, dist_dir: Path, dry_run: bool) -> None:
    archives = sorted(dist_dir.glob("foxxycode_*.tar.gz")) + sorted(dist_dir.glob("foxxycode_*.zip"))
    if not archives:
        return
    sums_path = dist_dir / "SHA256SUMS"
    lines = []
    for arc in archives:
        if dry_run:
            lines.append(f"<hash>  {arc.name}")
        else:
            lines.append(f"{sha256_file(arc)}  {arc.name}")
    if dry_run:
        ui.info(f"SHA256SUMS (dry-run): {sums_path}")
        return
    sums_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    ui.ok(f"Контрольные суммы: {sums_path}")


def build_target_cli(ui: UI, opts: BuildOptions) -> None:
    tags = opts.tags
    if opts.preset:
        tags = tags_from_preset(opts.preset)
    tags = normalize_tags(tags)

    require_tools(ui, ["go"])
    if which("git"):
        version = git_version()
    else:
        ui.warn("Git не найден — версия сборки: dev")
        version = "dev"

    if tags_need_ui_build(tags):
        ui_build(ui, opts.dry_run)

    targets: list[tuple[str, str]] = []
    if opts.all_release:
        targets = list(RELEASE_TARGETS)
    elif opts.goos and opts.goarch:
        targets = [(opts.goos, opts.goarch)]
    else:
        targets = [host_platform()]

    dist_dir = REPO_ROOT / "dist"
    if opts.all_release or opts.archive:
        if not opts.dry_run:
            shutil.rmtree(dist_dir, ignore_errors=True)
            dist_dir.mkdir(parents=True, exist_ok=True)

    built: list[Path] = []
    for goos, goarch in targets:
        ui.info(f"Сборка CLI: {goos}/{goarch} теги=[{', '.join(tags) or 'нет'}]")
        if opts.all_release or opts.archive:
            stem = f"foxxycode_{version}_{goos}_{goarch}"
            staging = dist_dir / "_staging" / stem
            out = staging / bin_name(goos)
        else:
            out = REPO_ROOT / "build" / bin_name(goos)

        go_build_one(
            ui,
            goos=goos,
            goarch=goarch,
            tags=tags,
            version=version,
            out_path=out,
            strip=opts.ldflags_strip,
            dry_run=opts.dry_run,
        )
        built.append(out)

        if opts.all_release or opts.archive:
            if goos == "windows":
                arc = dist_dir / f"{stem}.zip"
            else:
                arc = dist_dir / f"{stem}.tar.gz"
            if not opts.dry_run and out.is_file():
                pack_archive(ui, out, arc, goos, dry_run=False)
            elif opts.dry_run:
                pack_archive(ui, out, arc, goos, dry_run=True)

    if opts.all_release or opts.archive:
        staging_root = dist_dir / "_staging"
        if not opts.dry_run and staging_root.exists():
            shutil.rmtree(staging_root)
        write_sha256sums(ui, dist_dir, opts.dry_run)

    for p in built:
        if not opts.dry_run:
            ui.ok(f"Бинарник: {p.resolve()}")
        else:
            ui.ok(f"Бинарник (dry-run): {p}")


def java_major_version() -> Optional[int]:
    java = which("java")
    if not java:
        return None
    try:
        r = subprocess.run(
            [java, "-version"],
            capture_output=True,
            text=True,
            check=False,
        )
        text = (r.stderr or "") + (r.stdout or "")
        m = re.search(r'version "(\d+)', text)
        if m:
            return int(m.group(1))
        m = re.search(r'version "1\.(\d+)', text)
        if m:
            return int(m.group(1))
    except OSError:
        pass
    return None


def java_major_version_of(jbr_bin: Path) -> Optional[int]:
    """Probe a specific java executable for its major version."""
    java = str(jbr_bin / ("java.exe" if platform.system() == "Windows" else "java"))
    if not Path(java).is_file():
        return None
    try:
        r = subprocess.run(
            [java, "-version"],
            capture_output=True,
            text=True,
            check=False,
        )
        text = (r.stderr or "") + (r.stdout or "")
        m = re.search(r'version "(\d+)', text)
        if m:
            return int(m.group(1))
        m = re.search(r'version "1\.(\d+)', text)
        if m:
            return int(m.group(1))
    except OSError:
        pass
    return None


# Known install locations of JetBrains IDEs that ship a bundled JDK (JBR).
# Each entry is a glob rooted at a base directory; matching directories may
# contain a `jbr` (Windows/Linux) or `jbr/Contents/Home` (macOS) subtree.
JETBRAINS_IDE_PATTERNS: list[str] = [
    "IntelliJ*", "IDEA*", "PyCharm*", "WebStorm*", "GoLand*", "RubyMine*",
    "PhpStorm*", "Rider*", "CLion*", "DataGrip*", "AppCode*", "Android Studio*",
]


def _jbr_candidates_windows() -> list[Path]:
    candidates: list[Path] = []
    bases: list[Path] = [
        Path(os.environ.get("ProgramFiles", r"C:\Program Files")) / "JetBrains",
        Path(os.environ.get("ProgramFiles(x86)", r"C:\Program Files (x86)")) / "JetBrains",
        Path(os.environ.get("LOCALAPPDATA", "")) / "JetBrains" / "Toolbox" / "apps",
        Path(os.environ.get("LOCALAPPDATA", "")) / "Programs",
    ]
    for base in bases:
        if not base.exists():
            continue
        for ide_pattern in JETBRAINS_IDE_PATTERNS:
            for ide_dir in base.glob(ide_pattern):
                if ide_dir.is_dir():
                    # Toolbox layout: apps/<IDE>/ch-0/<version>/<IDE.app>/jbr
                    if base.name == "apps":
                        for ch in ide_dir.glob("ch-0/*"):
                            for app in ch.glob("*.app"):
                                yield_jbr = app / "jbr"
                                if yield_jbr.is_dir():
                                    candidates.append(yield_jbr)
                    else:
                        jbr = ide_dir / "jbr"
                        if jbr.is_dir():
                            candidates.append(jbr)
    # Bounded disk scan: look one level deep under each fixed drive root for a
    # `JetBrains` folder (covers custom install paths like D:\JetBrains\...).
    if platform.system() == "Windows":
        import string
        for letter in string.ascii_uppercase:
            drive = Path(f"{letter}:\\")
            if not drive.exists():
                continue
            jb = drive / "JetBrains"
            if jb.is_dir():
                for ide_pattern in JETBRAINS_IDE_PATTERNS:
                    for ide_dir in jb.glob(ide_pattern):
                        jbr = ide_dir / "jbr"
                        if jbr.is_dir():
                            candidates.append(jbr)
    return candidates


def _jbr_candidates_macos() -> list[Path]:
    candidates: list[Path] = []
    bases: list[Path] = [
        Path("/Applications"),
        Path.home() / "Applications" / "JetBrains" / "Toolbox",
        Path.home() / "Library" / "Application Support" / "JetBrains" / "Toolbox" / "apps",
    ]
    for base in bases:
        if not base.exists():
            continue
        for ide_pattern in JETBRAINS_IDE_PATTERNS:
            for app in base.glob(f"{ide_pattern}.app"):
                jbr = app / "Contents" / "jbr" / "Contents" / "Home"
                if jbr.is_dir():
                    candidates.append(jbr)
                # Some older layouts put jbr directly under Contents.
                jbr_alt = app / "Contents" / "jbr"
                if (jbr_alt / "bin").is_dir():
                    candidates.append(jbr_alt)
        # Toolbox: apps/<IDE>/ch-0/<version>/<IDE>.app/...
        if base.name == "apps":
            for ide_dir in base.iterdir():
                if not ide_dir.is_dir():
                    continue
                for ch in ide_dir.glob("ch-0/*"):
                    for app in ch.glob("*.app"):
                        jbr = app / "Contents" / "jbr" / "Contents" / "Home"
                        if jbr.is_dir():
                            candidates.append(jbr)
    return candidates


def _jbr_candidates_linux() -> list[Path]:
    candidates: list[Path] = []
    bases: list[Path] = [
        Path("/opt"),
        Path("/opt/jetbrains"),
        Path.home() / ".local" / "share" / "JetBrains" / "Toolbox" / "apps",
        Path.home() / ".local" / "share" / "JetBrains",
        Path("/usr/share"),
    ]
    for base in bases:
        if not base.exists():
            continue
        for ide_pattern in JETBRAINS_IDE_PATTERNS:
            for ide_dir in base.glob(ide_pattern):
                if ide_dir.is_dir():
                    jbr = ide_dir / "jbr"
                    if jbr.is_dir():
                        candidates.append(jbr)
        # Toolbox layout: apps/<IDE>/ch-0/<version>/<IDE>.app/Contents/jbr/...
        if base.name == "apps":
            for ide_dir in base.iterdir():
                if not ide_dir.is_dir():
                    continue
                for ch in ide_dir.glob("ch-0/*"):
                    for app in ch.glob("*.app"):
                        jbr = app / "Contents" / "jbr" / "Contents" / "Home"
                        if jbr.is_dir():
                            candidates.append(jbr)
                    # Plain Linux toolbox may lay out jbr directly.
                    for jbr in ch.glob("jbr"):
                        if (jbr / "bin").is_dir():
                            candidates.append(jbr)
    return candidates


def _jbr_ide_label(jbr: Path) -> str:
    """Best-effort human label for the IDE that owns a JBR path."""
    system = platform.system()
    try:
        if system == "Darwin":
            # .../<IDE>.app/Contents/jbr/Contents/Home or .../<IDE>.app/Contents/jbr
            parts = jbr.parts
            for i, p in enumerate(parts):
                if p.endswith(".app"):
                    return p[:-4]
            return jbr.parent.parent.parent.name
        # Windows/Linux: .../<IDE>/jbr or .../<IDE>.app/jbr
        if jbr.name in ("Home", "jbr"):
            return jbr.parent.name
        return jbr.parent.name
    except Exception:
        return str(jbr)


def find_jetbrains_jbrs(min_major: int = 17) -> list[tuple[Path, int]]:
    """Return all detected JetBrains JBR installations with Java >= min_major.

    Returns a list of (jbr_home, java_major) tuples, deduplicated and preserving
    discovery order (Program Files first, then Toolbox, then disk scan).
    """
    system = platform.system()
    if system == "Windows":
        raw = _jbr_candidates_windows()
    elif system == "Darwin":
        raw = _jbr_candidates_macos()
    else:
        raw = _jbr_candidates_linux()

    seen: set[Path] = set()
    out: list[tuple[Path, int]] = []
    for cand in raw:
        if cand in seen:
            continue
        seen.add(cand)
        bin_dir = cand / "bin"
        if not bin_dir.is_dir():
            continue
        major = java_major_version_of(bin_dir)
        if major is not None and major >= min_major:
            out.append((cand, major))
    return out


def choose_jbr(ui: UI, found: list[tuple[Path, int]]) -> Optional[Path]:
    """Interactive picker when several JetBrains JBRs are detected."""
    if len(found) == 1:
        return found[0][0]
    print()
    print("Найдено несколько JetBrains IDE с JBR 17+. Выберите JAVA_HOME для Gradle:")
    options: list[tuple[str, str]] = []
    for i, (jbr, major) in enumerate(found, start=1):
        label = _jbr_ide_label(jbr)
        options.append((str(i), f"{label} — Java {major}\n     {jbr}"))
    # Default to the newest Java (then the first found).
    default_idx = max(range(len(found)), key=lambda i: found[i][1])
    default_key = str(default_idx + 1)
    choice = prompt_choice(
        ui,
        "Выберите JBR:",
        options,
        default=default_key,
    )
    idx = int(choice) - 1
    if 0 <= idx < len(found):
        return found[idx][0]
    return None


def resolve_java_home(ui: UI, dry_run: bool) -> Optional[Path]:
    """Resolve a JDK 17+ for the IntelliJ build.

    Order: explicit $JAVA_HOME -> java on PATH -> auto-detected JetBrains JBR
    (with interactive choice when several IDEs are installed).
    Returns the JDK home directory, or None if nothing suitable was found.
    """
    explicit = os.environ.get("JAVA_HOME")
    if explicit:
        explicit_path = Path(explicit)
        bin_dir = explicit_path / "bin"
        major = java_major_version_of(bin_dir) if bin_dir.is_dir() else None
        if major is not None:
            ui.info(f"JAVA_HOME: {explicit_path} (Java {major})")
            if major >= 17:
                return explicit_path
            ui.warn(f"JAVA_HOME указывает на Java {major}, нужен 17+ — ищу JBR от JetBrains.")

    on_path = java_major_version()
    if on_path is not None and on_path >= 17:
        # java is on PATH and suitable; let Gradle use it directly.
        ui.info(f"Использую java из PATH (Java {on_path}).")
        return None

    ui.info("Поиск JBR (JetBrains Runtime) в установленных IDE...")
    found = find_jetbrains_jbrs(min_major=17)
    for cand, major in found:
        ui.info(f"Найден JBR: {cand} (Java {major})")

    if not found:
        if not dry_run:
            ui.err("JDK 17+ не найден ни в JAVA_HOME, ни на PATH, ни среди IDE JetBrains.")
            if platform.system() == "Windows":
                ui.info(
                    "Установите IntelliJ IDEA / PyCharm / WebStorm и т.п. (JBR 17 входит в комплект), "
                    'либо задайте JAVA_HOME вручную, например:\n'
                    '  PowerShell: $env:JAVA_HOME = "C:\\Program Files\\JetBrains\\<IDE>\\jbr"\n'
                    "  либо установите Temurin JDK 17: https://adoptium.net/"
                )
            else:
                ui.info(
                    "Установите JDK 17+ (Temurin: https://adoptium.net/) или укажите "
                    "существующий JBR: export JAVA_HOME=/path/to/jbr"
                )
        return None

    if len(found) == 1:
        jbr = found[0][0]
    elif dry_run:
        # Non-interactive / CI: take the highest Java version, then the first.
        jbr = max(found, key=lambda x: (x[1], -found.index(x)))[0]
        ui.info(f"Dry-run: автоматически выбран {jbr} (из {len(found)} кандидатов).")
    else:
        jbr = choose_jbr(ui, found)

    if jbr is not None:
        ui.ok(f"Использую JBR: {jbr}")
    return jbr



def intellij_gradle_cmd(version: str, production: bool) -> tuple[list[str], Path, bool]:
    ij_dir = REPO_ROOT / "editors" / "intellij"
    if platform.system() == "Windows":
        gradlew = ij_dir / "gradlew.bat"
        args = [
            str(gradlew),
            "--no-daemon",
            "buildPlugin",
            f"-PpluginVersion={version}",
        ]
        if production:
            args.append("-Pproduction=true")
        return args, ij_dir, False
    if which("make"):
        args = ["make", "intellij-build", f"PLUGIN_VERSION={version}"]
        return args, REPO_ROOT, False
    gradlew = ij_dir / "gradlew"
    args = [
        str(gradlew),
        "--no-daemon",
        "buildPlugin",
        f"-PpluginVersion={version}",
    ]
    if production:
        args.append("-Pproduction=true")
    return args, ij_dir, False


def build_target_intellij(ui: UI, opts: BuildOptions) -> None:
    require_tools(ui, ["go", "npm"])

    jbr_home = resolve_java_home(ui, dry_run=opts.dry_run)
    if jbr_home is None and not opts.dry_run:
        raise SystemExit(1)

    version = opts.plugin_version or git_version()
    ui.info(f"Версия плагина: {version}")

    args, cwd, shell = intellij_gradle_cmd(version, opts.production)
    env_override: Optional[dict[str, str]] = None
    if jbr_home is not None:
        env_override = {"JAVA_HOME": str(jbr_home)}
        ui.info(f"JAVA_HOME для Gradle: {jbr_home}")

    try:
        run_cmd(ui, args, cwd=cwd, env=env_override, dry_run=opts.dry_run, shell=shell)
    except subprocess.CalledProcessError as e:
        ui.err(
            "Сборка IntelliJ-плагина завершилась с ошибкой Gradle "
            f"(код {e.returncode}). Частые причины:"
        )
        ui.info(
            "- Сетевой таймаут при скачивании IntelliJ IDEA SDK (~1 ГБ) с "
            "cache-redirector.jetbrains.com — перезапустите команду, частично "
            "скачанные артефалы кэшируются и повторная попытка обычно доуспевает."
        )
        ui.info(
            "- Прокси/файрвол блокируют https://cache-redirector.jetbrains.com — "
            "настройте HTTP_PROXY/HTTPS_PROXY или VPN."
        )
        ui.info(
            "- Подробный лог: добавьте --stacktrace --info к аргументам Gradle "
            "(передаётся через переменную GRADLE_OPTS или вручную в editors/intellij)."
        )
        raise SystemExit(1)

    dist_glob = REPO_ROOT / "editors" / "intellij" / "build" / "distributions"
    if opts.dry_run:
        ui.ok(f"Плагин (dry-run): {dist_glob}/*.zip")
        return
    zips = list(dist_glob.glob("*.zip"))
    if zips:
        for z in zips:
            ui.ok(f"Плагин: {z.resolve()}")
    else:
        ui.warn(f"ZIP не найден в {dist_glob}")


def parse_go_target(s: str) -> tuple[str, str]:
    parts = s.split("-", 1)
    if len(parts) != 2:
        raise ValueError(f"Неверный формат цели: {s} (ожидается goos-goarch)")
    return parts[0], parts[1]


def vsce_target(goos: str, goarch: str) -> str:
    key = (goos, goarch)
    if key not in GO_TO_VSCE:
        raise ValueError(f"Нет маппинга vsce для {goos}-{goarch}")
    return GO_TO_VSCE[key]


def build_target_vscode(ui: UI, opts: BuildOptions) -> None:
    require_tools(ui, ["go", "node", "npm"])
    ui.warn(
        "Расширение VS Code — scaffold (см. editors/vscode/README.md); "
        "сборка VSIX подготовительная."
    )

    vs_dir = REPO_ROOT / "editors" / "vscode"
    if not vs_dir.is_dir():
        ui.err(f"Каталог не найден: {vs_dir}")
        raise SystemExit(1)

    targets: list[tuple[str, str]] = []
    if opts.vscode_targets:
        for t in opts.vscode_targets:
            targets.append(parse_go_target(t))
    else:
        targets = [host_platform()]

    npm = npm_cmd()
    if not opts.dry_run:
        run_cmd(ui, [npm, "install", "--no-fund", "--no-audit"], cwd=vs_dir, dry_run=False)

    for goos, goarch in targets:
        go_target = f"{goos}-{goarch}"
        vsce = vsce_target(goos, goarch)
        ui.info(f"VS Code: {go_target} -> vsce --target {vsce}")

        run_cmd(
            ui,
            ["node", "scripts/prepare-binary.mjs", "--target", go_target],
            cwd=vs_dir,
            dry_run=opts.dry_run,
        )
        run_cmd(
            ui,
            [npm, "run", "compile"],
            cwd=vs_dir,
            dry_run=opts.dry_run,
        )
        run_cmd(
            ui,
            [npm, "exec", "--", "vsce", "package", "--target", vsce],
            cwd=vs_dir,
            dry_run=opts.dry_run,
        )

    if not opts.dry_run:
        vsix = list(vs_dir.glob("*.vsix"))
        for v in vsix:
            ui.ok(f"VSIX: {v.resolve()}")
    else:
        ui.ok(f"VSIX (dry-run): {vs_dir}/*.vsix")


def build_target_all(ui: UI, opts: BuildOptions) -> None:
    cli_opts = BuildOptions(
        target="cli",
        tags=opts.tags,
        preset=opts.preset or "full",
        all_release=True,
        archive=True,
        ldflags_strip=opts.ldflags_strip,
        dry_run=opts.dry_run,
        no_color=opts.no_color,
    )
    build_target_cli(ui, cli_opts)

    ij_opts = BuildOptions(
        target="intellij",
        plugin_version=opts.plugin_version,
        production=opts.production,
        dry_run=opts.dry_run,
        no_color=opts.no_color,
    )
    build_target_intellij(ui, ij_opts)

    vs_opts = BuildOptions(
        target="vscode",
        vscode_targets=[f"{g}-{a}" for g, a in RELEASE_TARGETS],
        dry_run=opts.dry_run,
        no_color=opts.no_color,
    )
    build_target_vscode(ui, vs_opts)


def prompt_choice(ui: UI, title: str, options: list[tuple[str, str]], default: str = "1") -> str:
    print()
    print(title)
    for key, label in options:
        mark = " <- по умолчанию" if key == default else ""
        print(f"  {key}) {label}{mark}")
    while True:
        raw = input("> ").strip()
        if not raw:
            raw = default
        if any(k == raw for k, _ in options):
            return raw
        ui.warn("Неверный выбор, повторите.")


def prompt_tags(ui: UI) -> list[str]:
    choice = prompt_choice(
        ui,
        "Теги/плагины (пресеты):",
        [
            ("1", "Lean — только ACP, без http/UI/scheduler/memory"),
            ("2", "Full — http ui scheduler memory (как Docker/релизы)"),
            ("3", "Gateway — http ui scheduler memory gateway.telegram"),
            ("4", "Свои теги — выбрать вручную"),
        ],
        default="2",
    )
    if choice == "1":
        return []
    if choice == "2":
        return tags_from_preset("full")
    if choice == "3":
        return tags_from_preset("gateway")
    raw = input(
        "Включить теги (через запятую из: http, ui, scheduler, memory, gateway.telegram, gateway): "
    ).strip()
    try:
        return normalize_tags([t.strip() for t in raw.split(",") if t.strip()])
    except ValueError as e:
        ui.err(str(e))
        raise SystemExit(1)


def prompt_platform(ui: UI) -> tuple[list[tuple[str, str]], bool, bool]:
    host = host_platform()
    choice = prompt_choice(
        ui,
        "Платформа:",
        [
            ("1", f"Текущая ({host[0]}/{host[1]})"),
            ("2", "linux/amd64"),
            ("3", "linux/arm64"),
            ("4", "darwin/amd64 (macOS Intel)"),
            ("5", "darwin/arm64 (macOS Apple Silicon)"),
            ("6", "windows/amd64"),
            ("7", "Все платформы релиза (5 целей + архивы + SHA256SUMS)"),
        ],
        default="1",
    )
    mapping = {
        "1": [host],
        "2": [("linux", "amd64")],
        "3": [("linux", "arm64")],
        "4": [("darwin", "amd64")],
        "5": [("darwin", "arm64")],
        "6": [("windows", "amd64")],
        "7": list(RELEASE_TARGETS),
    }
    targets = mapping[choice]
    all_release = choice == "7"
    return targets, all_release, all_release


def interactive_menu(ui: UI) -> None:
    host = host_platform()
    print("=== Мастер сборки FoxxyCode ===")
    ui.info(f"Текущая платформа: {host[0]}/{host[1]} (host)")
    ui.info(f"Корень репозитория: {REPO_ROOT}")

    main = prompt_choice(
        ui,
        "Выберите цель сборки:",
        [
            ("1", "CLI-бинарник foxxycode (standalone)"),
            ("2", "Плагин IntelliJ (JetBrains IDE)"),
            ("3", "Расширение VS Code (VSIX)"),
            ("4", "Всё сразу (CLI для всех платформ + оба плагина)"),
            ("0", "Выход"),
        ],
        default="1",
    )
    if main == "0":
        ui.info("Выход.")
        return

    dry = prompt_choice(
        ui,
        "Режим:",
        [("1", "Реальная сборка"), ("2", "Только показать команды (dry-run)")],
        default="1",
    )
    dry_run = dry == "2"

    if main == "1":
        tags = prompt_tags(ui)
        targets, all_release, archive = prompt_platform(ui)
        strip = prompt_choice(
            ui,
            "Оптимизация бинарника (-s -w):",
            [("1", "Нет (как make build)"), ("2", "Да (как release CI)")],
            default="1",
        )
        for goos, goarch in targets:
            opts = BuildOptions(
                target="cli",
                tags=tags,
                goos=goos,
                goarch=goarch,
                all_release=all_release,
                archive=archive,
                ldflags_strip=strip == "2",
                dry_run=dry_run,
            )
            if all_release:
                opts.all_release = True
                build_target_cli(ui, opts)
                break
            build_target_cli(ui, opts)

    elif main == "2":
        ver = input("Версия плагина (Enter = из git): ").strip()
        prod = prompt_choice(
            ui,
            "Режим Gradle:",
            [
                ("1", "Production (-Pproduction=true, все платформы)"),
                ("2", "Dev (только host-платформа)"),
            ],
            default="1",
        )
        opts = BuildOptions(
            target="intellij",
            plugin_version=ver,
            production=prod == "1",
            dry_run=dry_run,
        )
        build_target_intellij(ui, opts)

    elif main == "3":
        plat = prompt_choice(
            ui,
            "Платформа VSIX:",
            [
                ("1", f"Текущая ({host[0]}-{host[1]})"),
                ("2", "Все платформы релиза (5 VSIX)"),
            ],
            default="1",
        )
        if plat == "2":
            vs_targets = [f"{g}-{a}" for g, a in RELEASE_TARGETS]
        else:
            vs_targets = [f"{host[0]}-{host[1]}"]
        opts = BuildOptions(target="vscode", vscode_targets=vs_targets, dry_run=dry_run)
        build_target_vscode(ui, opts)

    elif main == "4":
        ver = input("Версия плагина IntelliJ (Enter = из git): ").strip()
        opts = BuildOptions(
            target="all",
            preset="full",
            plugin_version=ver,
            production=True,
            dry_run=dry_run,
        )
        build_target_all(ui, opts)


def build_arg_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        description="Универсальный мастер сборки FoxxyCode Agent (CLI, IntelliJ, VS Code).",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Примеры:
  python scripts/build.py
  python scripts/build.py --target cli --preset full
  python scripts/build.py --target cli --goos linux --goarch arm64 --tags http,scheduler --dry-run
  python scripts/build.py --target cli --all-release --preset full --ldflags-strip
  python scripts/build.py --target intellij --plugin-version 1.2.3 --production
  python scripts/build.py --target vscode --vscode-target darwin-arm64
  python scripts/build.py --target all --preset full
""",
    )
    p.add_argument(
        "--target",
        choices=["cli", "intellij", "vscode", "all"],
        help="Цель сборки (без флага — интерактивное меню).",
    )
    p.add_argument(
        "--preset",
        choices=list(PRESETS.keys()),
        help="Пресет тегов: lean, full, gateway.",
    )
    p.add_argument(
        "--tags",
        help="Свои теги через запятую (http,ui,scheduler,memory,gateway.telegram,gateway).",
    )
    p.add_argument("--goos", help="GOOS для CLI (linux, darwin, windows).")
    p.add_argument("--goarch", help="GOARCH для CLI (amd64, arm64).")
    p.add_argument(
        "--all-release",
        action="store_true",
        help="Собрать все 5 платформ релиза + архивы + SHA256SUMS.",
    )
    p.add_argument(
        "--archive",
        action="store_true",
        help="Упаковать бинарник в dist/ (tar.gz или zip).",
    )
    p.add_argument(
        "--ldflags-strip",
        action="store_true",
        help="Добавить -s -w к ldflags (как release CI).",
    )
    p.add_argument("--plugin-version", help="Версия IntelliJ-плагина (по умолчанию из git).")
    p.add_argument(
        "--production",
        action="store_true",
        default=None,
        help="IntelliJ: production-сборка со всеми платформами (-Pproduction=true).",
    )
    p.add_argument(
        "--dev",
        action="store_true",
        help="IntelliJ: dev-сборка только для host-платформы.",
    )
    p.add_argument(
        "--vscode-target",
        action="append",
        dest="vscode_targets",
        metavar="GOOS-GOARCH",
        help="Цель VS Code (linux-amd64, darwin-arm64, …). Можно указать несколько раз.",
    )
    p.add_argument("--dry-run", action="store_true", help="Показать команды без выполнения.")
    p.add_argument("--no-color", action="store_true", help="Отключить ANSI-цвета.")
    return p


def opts_from_args(args: argparse.Namespace) -> BuildOptions:
    tags: list[str] = []
    if args.tags:
        tags = [t.strip() for t in args.tags.split(",") if t.strip()]
    production = True
    if args.dev:
        production = False
    if args.production:
        production = True

    return BuildOptions(
        target=args.target or "",
        tags=tags,
        preset=args.preset or "",
        goos=args.goos or "",
        goarch=args.goarch or "",
        all_release=args.all_release,
        archive=args.archive,
        ldflags_strip=args.ldflags_strip,
        plugin_version=args.plugin_version or "",
        production=production,
        vscode_targets=args.vscode_targets or [],
        dry_run=args.dry_run,
        no_color=args.no_color,
    )


def run_noninteractive(ui: UI, opts: BuildOptions) -> None:
    try:
        if opts.tags:
            opts.tags = normalize_tags(opts.tags)
    except ValueError as e:
        ui.err(str(e))
        raise SystemExit(1)

    dispatch: dict[str, Callable[[UI, BuildOptions], None]] = {
        "cli": build_target_cli,
        "intellij": build_target_intellij,
        "vscode": build_target_vscode,
        "all": build_target_all,
    }
    fn = dispatch.get(opts.target)
    if fn is None:
        ui.err(f"Неизвестная цель: {opts.target}")
        raise SystemExit(1)
    fn(ui, opts)


def main() -> None:
    configure_stdio()
    os.chdir(REPO_ROOT)

    parser = build_arg_parser()
    args = parser.parse_args()
    use_color = not args.no_color and sys.stdout.isatty()
    ui = UI(use_color=use_color)

    if args.target:
        opts = opts_from_args(args)
        run_noninteractive(ui, opts)
    else:
        interactive_menu(ui)


if __name__ == "__main__":
    main()
