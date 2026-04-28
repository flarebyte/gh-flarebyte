ORG="my-org"
CURSOR=null

while :; do
  RESP=$(gh api graphql -f query='
  query($cursor: String) {
    viewer {
      repositoriesContributedTo(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          nameWithOwner
          owner { login }
        }
      }
    }
  }' -F cursor="$CURSOR")

  echo "$RESP" | jq -r --arg org "$ORG" '
    .data.viewer.repositoriesContributedTo.nodes[]
    | select(.owner.login == $org)
    | .nameWithOwner
  '

  HAS_NEXT=$(echo "$RESP" | jq -r '.data.viewer.repositoriesContributedTo.pageInfo.hasNextPage')
  CURSOR=$(echo "$RESP" | jq -r '.data.viewer.repositoriesContributedTo.pageInfo.endCursor')

  [ "$HAS_NEXT" != "true" ] && break
done | sort -u