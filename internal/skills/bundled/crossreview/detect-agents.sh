#!/bin/sh
# detect-agents.sh - probe which console code-agent CLIs are installed.
#
# Prints one line per detected agent, tab-separated:
#   agent <TAB> binary_path <TAB> models_cmd <TAB> run_template
#
# run_template uses {model}, {brief} and {out} placeholders; the caller bakes
# {model} in at roster-write time and the coordinator substitutes {brief} and
# {out} per run. models_cmd is empty when the CLI cannot list its own models
# and the user has to type them.

set -u

# timeout(1) is GNU coreutils and stock macOS does not ship it; probe falls
# back to running the command directly when it is absent.
have_timeout=0
if command -v timeout >/dev/null 2>&1; then
    have_timeout=1
fi

probe() {
    # probe <seconds> <cmd...>
    _secs="$1"; shift
    if [ "$have_timeout" = 1 ]; then
        timeout "$_secs" "$@"
    else
        "$@"
    fi
}

# verify <binary> <marker> - the name resolving is not enough (command -v
# agent can name an unrelated executable); the binary must run --version or
# --help successfully.
verify() {
    _bin="$1"
    if command -v "$_bin" >/dev/null 2>&1; then
        _path="$(command -v "$_bin")"
        if probe 10 "$_path" --version >/dev/null 2>&1 || probe 10 "$_path" --help >/dev/null 2>&1; then
            return 0
        fi
    fi
    return 1
}

# models_ok <cmd...> - the listing command works when it exits 0 quickly.
models_ok() {
    probe 20 "$@" >/dev/null 2>&1
}

emit() {
    # emit <agent> <path> <models_cmd> <run_template>
    printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4"
}

if verify claude; then
    emit claude "$(command -v claude)" "" \
        'claude -p --model {model} --output-format text --permission-mode plan < {brief} > {out}'
fi

if verify codex; then
    _models=""
    if models_ok codex debug models; then _models="codex debug models"; fi
    emit codex "$(command -v codex)" "$_models" \
        'codex exec -m {model} --sandbox read-only - < {brief} > {out}'
fi

if verify foxxycode; then
    emit foxxycode "$(command -v foxxycode)" "" \
        'foxxycode -p -i {brief} --model {model} --mode ask > {out}'
fi

if verify opencode; then
    _models=""
    if models_ok opencode models; then _models="opencode models"; fi
    emit opencode "$(command -v opencode)" "$_models" \
        'opencode run -m {model} "$(cat {brief})" > {out}'
fi

# cursor's binary is cursor-agent; agent/cursor are aliases for it.
for _b in cursor-agent agent cursor; do
    if verify "$_b"; then
        _models=""
        if models_ok "$_b" --list-models; then _models="$_b --list-models"; fi
        emit cursor "$(command -v "$_b")" "$_models" \
            "$_b -p --mode ask --model {model} --output-format text \"\$(cat {brief})\" > {out}"
        break
    fi
done

if verify devin; then
    _models=""
    if models_ok devin models list; then _models="devin models list"; fi
    emit devin "$(command -v devin)" "$_models" \
        'devin -p --model {model} --prompt-file {brief} > {out}'
fi

if verify koda; then
    emit koda "$(command -v koda)" "" \
        'koda "$(cat {brief})" > {out}'
fi
