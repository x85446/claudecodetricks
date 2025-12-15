#!/bin/bash
# agent2clip.sh - Extract agents from team.md and copy each to clipboard
# Usage: agent2clip.sh path/to/team.md

set -e

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <path/to/team.md>"
    exit 1
fi

TEAM_FILE="$1"

if [[ ! -f "$TEAM_FILE" ]]; then
    echo "Error: File not found: $TEAM_FILE"
    exit 1
fi

# Extract agent sections
# Agents start with headers like:
#   ### Orchestrator: Name
#   #### Staff Engineer: Name
#   #### Manager: Name
#   #### Specialist: Name
# Sections end at --- or next agent header

extract_agents() {
    local in_agent=0
    local agent_name=""
    local agent_content=""
    local line_num=0

    while IFS= read -r line || [[ -n "$line" ]]; do
        line_num=$((line_num + 1))

        # Check if this is an agent header
        if [[ "$line" =~ ^###[[:space:]]+(Orchestrator|Staff\ Engineer|Manager|Specialist):[[:space:]]+ ]] || \
           [[ "$line" =~ ^####[[:space:]]+(Staff\ Engineer|Manager|Specialist):[[:space:]]+ ]]; then

            # If we were already collecting an agent, output it
            if [[ $in_agent -eq 1 && -n "$agent_content" ]]; then
                echo "===AGENT_START===$agent_name===AGENT_CONTENT==="
                printf "%s" "$agent_content"
                echo "===AGENT_END==="
            fi

            # Start new agent
            in_agent=1
            agent_name=$(echo "$line" | sed -E 's/^#{3,4}[[:space:]]+(Orchestrator|Staff Engineer|Manager|Specialist):[[:space:]]+//')
            agent_content="$line"$'\n'

        elif [[ $in_agent -eq 1 ]]; then
            # Check for section end
            if [[ "$line" == "---" ]]; then
                # Output current agent
                if [[ -n "$agent_content" ]]; then
                    echo "===AGENT_START===$agent_name===AGENT_CONTENT==="
                    printf "%s" "$agent_content"
                    echo "===AGENT_END==="
                fi
                in_agent=0
                agent_name=""
                agent_content=""
            else
                # Accumulate content
                agent_content+="$line"$'\n'
            fi
        fi
    done < "$TEAM_FILE"

    # Output last agent if file doesn't end with ---
    if [[ $in_agent -eq 1 && -n "$agent_content" ]]; then
        echo "===AGENT_START===$agent_name===AGENT_CONTENT==="
        printf "%s" "$agent_content"
        echo "===AGENT_END==="
    fi
}

# Parse the output and copy each agent to clipboard
agent_count=0
current_agent=""
current_content=""
in_content=0

while IFS= read -r line; do
    if [[ "$line" =~ ^===AGENT_START===(.*)===AGENT_CONTENT===$ ]]; then
        current_agent="${BASH_REMATCH[1]}"
        current_content=""
        in_content=1
    elif [[ "$line" == "===AGENT_END===" ]]; then
        if [[ -n "$current_content" ]]; then
            agent_count=$((agent_count + 1))
            echo "[$agent_count] Copying: $current_agent"
            printf "%s" "$current_content" | pbcopy
            sleep 1
        fi
        in_content=0
        current_agent=""
        current_content=""
    elif [[ $in_content -eq 1 ]]; then
        current_content+="$line"$'\n'
    fi
done < <(extract_agents)

echo ""
echo "Done! Copied $agent_count agents to clipboard (1 second apart)"
