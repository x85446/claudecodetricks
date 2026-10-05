# box.sh — the one box builder for the tutorial runtime.
#
# Sourced by run.sh and tutorial.sh from their own directory, and copied
# verbatim beside them into a project's docs/tutorials/. Never rewritten per
# project. Runs under macOS /bin/bash 3.2.
#
#   tut_box <colour> <title> [row…]
#
# <colour> is the SGR sequence for the frame and title ("" for none). Rows may
# carry their own SGR colours: width is measured with them stripped, so every
# line ends in the right border whatever it contains. The frame is as wide as
# its widest row (or title), capped at the terminal width — COLUMNS, else the
# terminal stdout writes to, else `tput cols`, else 80 — and a longer row wraps
# at word boundaries inside it,
# continuation lines keeping the row's leading indent. Outside a UTF-8 locale,
# or on TERM=dumb, the frame is drawn in + - |.
#
# It also owns the two terminal decisions both scripts share, so they are made
# in one place: tut_color_on (colour or not) and tut_utf8 (symbols or ASCII).

_tb_esc=$'\033'

# 0 when stdout gets colour. The ladder, first match wins: NO_COLOR non-empty →
# off; FORCE_COLOR, or CLICOLOR_FORCE other than 0 → on, even piped;
# TERM=dumb → off; stdout not a terminal → off; else on.
tut_color_on() {
    [ -n "${NO_COLOR:-}" ] && return 1
    [ -n "${FORCE_COLOR:-}" ] && return 0
    [ -n "${CLICOLOR_FORCE:-}" ] && [ "$CLICOLOR_FORCE" != 0 ] && return 0
    [ "${TERM:-dumb}" = dumb ] && return 1
    [ -t 1 ]
}

# 0 when the terminal takes UTF-8 symbols: a UTF-8 locale, and TERM not dumb.
tut_utf8() {
    [ "${TERM:-dumb}" = dumb ] && return 1
    case "${LC_ALL:-${LC_CTYPE:-${LANG:-}}}" in
        *[Uu][Tt][Ff]-8*|*[Uu][Tt][Ff]8*) return 0 ;;
    esac
    return 1
}

# $1 with every SGR sequence removed → _tb_plain
_tut_box_plain() {
    local s="$1" out=""
    while :; do
        case "$s" in
            *"$_tb_esc["*)
                out="$out${s%%"$_tb_esc["*}"
                s="${s#*"$_tb_esc["}"
                s="${s#*m}"
                ;;
            *) break ;;
        esac
    done
    _tb_plain="$out$s"
}

# Display columns of $1 → _tb_w. One column per character: UTF-8 continuation
# bytes are dropped first, which counts characters in a C locale too.
_tut_box_width() {
    _tut_box_plain "$1"
    local t="${_tb_plain//[$'\x80'-$'\xbf']/}"
    _tb_w=${#t}
}

# $2 copies of the string $1 → _tb_rep
_tut_box_rep() {
    local n="$2" s=""
    while [ "$n" -gt 0 ]; do s="$s$1"; n=$((n - 1)); done
    _tb_rep="$s"
}

# The SGR in effect after $1, given $2 in effect before it → _tb_sgr
_tut_box_sgr_after() {
    local word="$1" last p
    _tb_sgr="$2"
    case "$word" in *"$_tb_esc["*) ;; *) return ;; esac
    last="${word##*"$_tb_esc["}"
    p="${last%%m*}"
    if [ -z "$p" ] || [ "$p" = 0 ]; then _tb_sgr=""; else _tb_sgr="$_tb_esc[${p}m"; fi
}

# Row $1 wrapped to $2 columns at spaces → _tb_lines (array)
_tut_box_wrap() {
    local row="$1" max="$2" indent="" line lw word ww room carry="" plain chunk
    _tb_lines=()
    _tut_box_width "$row"
    if [ "$_tb_w" -le "$max" ]; then _tb_lines=("$row"); return; fi
    while [ "${row# }" != "$row" ]; do indent="$indent "; row="${row# }"; done
    [ "${#indent}" -ge "$max" ] && indent=""
    room=$((max - ${#indent}))
    line="$indent"; lw=${#indent}
    local words
    IFS=' ' read -r -a words <<< "$row"
    for word in ${words[@]+"${words[@]}"}; do
        _tut_box_width "$word"; ww=$_tb_w
        if [ "$ww" -gt "$room" ]; then
            # A word wider than the frame is cut by characters, colours dropped.
            [ "$lw" -gt "${#indent}" ] && _tb_lines+=("$line")
            _tut_box_plain "$word"; plain="$_tb_plain"
            while [ "${#plain}" -gt "$room" ]; do
                chunk="${plain:0:$room}"; plain="${plain:$room}"
                _tb_lines+=("$indent$chunk")
            done
            line="$indent$plain"; lw=$((${#indent} + ${#plain}))
            continue
        fi
        if [ "$lw" -eq "${#indent}" ]; then
            line="$line$word"; lw=$((lw + ww))
        elif [ $((lw + 1 + ww)) -le "$max" ]; then
            line="$line $word"; lw=$((lw + 1 + ww))
        else
            _tb_lines+=("$line")
            line="$indent$carry$word"; lw=$((${#indent} + ww))
        fi
        _tut_box_sgr_after "$word" "$carry"; carry="$_tb_sgr"
    done
    [ "$lw" -gt "${#indent}" ] && _tb_lines+=("$line")
    return 0
}

# Terminal width → _tb_cols: COLUMNS, else the terminal stdout writes to, else
# `tput cols`, else 80. Inside $(…) tput's stdout is a pipe and it answers its
# terminfo default, so stdout's terminal is asked first, through a dup on fd 3.
tut_box_cols() {
    local cols="${COLUMNS:-}"
    case "$cols" in ''|*[!0-9]*) { cols=$(stty size <&3 2>/dev/null); } 3>&1; cols="${cols#* }" ;; esac
    case "$cols" in ''|*[!0-9]*) cols=$(tput cols 2>/dev/null) ;; esac
    case "$cols" in ''|*[!0-9]*) cols=80 ;; esac
    [ "$cols" -lt 12 ] && cols=12
    _tb_cols=$cols
}

tut_box() {
    local c="$1" title="$2"; shift 2
    local o="" cols inner tw row w h v tl tr bl br line pad
    if tut_utf8; then h='─'; v='│'; tl='╭'; tr='╮'; bl='╰'; br='╯'
    else h='-'; v='|'; tl='+'; tr='+'; bl='+'; br='+'; fi
    tut_box_cols; cols=$_tb_cols

    # Frame: │ + 2 spaces + inner + 1 space + │. Top: ╭─ title ─…╮.
    _tut_box_width "$title"; tw=$_tb_w
    inner=0
    for row in ${1+"$@"}; do
        _tut_box_width "$row"; [ "$_tb_w" -gt "$inner" ] && inner=$_tb_w
    done
    [ "$tw" -gt 0 ] && [ $((tw + 1)) -gt "$inner" ] && inner=$((tw + 1))
    [ "$inner" -gt $((cols - 5)) ] && inner=$((cols - 5))
    if [ "$tw" -gt 0 ] && [ $((tw + 1)) -gt "$inner" ]; then
        _tut_box_plain "$title"
        if [ "$h" = '─' ]; then title="${_tb_plain:0:$((inner - 2))}…"
        else title="${_tb_plain:0:$((inner - 4))}..."; fi
        _tut_box_width "$title"; tw=$_tb_w
    fi

    case "$c$title$*" in *"$_tb_esc"*) o="$_tb_esc[0m" ;; esac

    if [ "$tw" -gt 0 ]; then
        _tut_box_rep "$h" $((inner - tw))
        printf '%s%s%s %s%s%s %s%s%s\n' "$c" "$tl" "$h" "$title" "$o" "$c" "$_tb_rep" "$tr" "$o"
    else
        _tut_box_rep "$h" $((inner + 3))
        printf '%s%s%s%s%s\n' "$c" "$tl" "$_tb_rep" "$tr" "$o"
    fi
    for row in ${1+"$@"}; do
        _tut_box_wrap "$row" "$inner"
        for line in ${_tb_lines[@]+"${_tb_lines[@]}"}; do
            _tut_box_width "$line"
            _tut_box_rep ' ' $((inner - _tb_w)); pad="$_tb_rep"
            printf '%s%s%s  %s%s%s %s%s%s\n' "$c" "$v" "$o" "$line" "$o" "$pad" "$c" "$v" "$o"
        done
    done
    _tut_box_rep "$h" $((inner + 3))
    printf '%s%s%s%s%s\n' "$c" "$bl" "$_tb_rep" "$br" "$o"
}
