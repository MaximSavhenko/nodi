# Enable with: source <(nodi completion bash)
_nodi_complete() {
    local line cur="${COMP_WORDS[COMP_CWORD]}"
    COMPREPLY=()
    while IFS= read -r line; do
        if [[ "$line" == ':files' ]]; then
            while IFS= read -r line; do COMPREPLY+=("$line"); done < <(compgen -f -- "$cur")
            return 0
        fi
        [[ -n "$line" ]] && COMPREPLY+=("$line")
    done < <("${COMP_WORDS[0]}" __complete "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null)
    return 0
}
complete -o filenames -F _nodi_complete nodi
