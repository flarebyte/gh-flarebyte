gh repo edit my-org/my-repo \
  --enable-issues=true \
  --enable-wiki=false \
  --enable-projects=false \
  --enable-discussions=false \
  --enable-squash-merge=true \
  --enable-merge-commit=false \
  --enable-rebase-merge=false \
  --delete-branch-on-merge=true \
  --enable-auto-merge=true \
  --default-branch main

gh repo edit my-org/my-repo \
  --enable-advanced-security \
  --enable-secret-scanning \
  --enable-secret-scanning-push-protection

gh api repos/{owner}/{repo} \
  --jq '.security_and_analysis.secret_scanning,
        .security_and_analysis.secret_scanning_push_protection'

# Basic label
gh label create bug --color FF0000 --description "Something is broken"

# Feature label
gh label create enhancement --color 00FF00 --description "New feature"

# Priority labels
gh label create "priority:high" --color B60205
gh label create "priority:medium" --color FBCA04
gh label create "priority:low" --color 0E8A16

# Force update if already exists
gh label create bug --color FF0000 --description "Something is broken" --force

# List all labels
gh label list

# JSON output (useful for scripting)
gh label list --json name,color,description

# Filter with jq
gh label list --json name,color | jq -r '.[] | select(.name | test("priority"))'

gh label view bug

# Rename label
gh label edit bug --name "type:bug"

# Update color
gh label edit bug --color FF5555

# Update description
gh label edit bug --description "Bug or defect"

# All at once
gh label edit bug \
  --name "type:bug" \
  --color FF5555 \
  --description "Bug or defect"

gh label delete bug

# Non-interactive (for scripts)
gh label delete bug --yes

# Basic
gh release create v1.2.3

# Specific repo
gh release create v1.2.3 \
  --repo my-org/my-repo

# Title
gh release create v1.2.3 \
  --title "v1.2.3"

# Inline notes
gh release create v1.2.3 \
  --notes "Bugfix release"

# Notes from file
gh release create v1.2.3 \
  --notes-file release-notes.md

# Notes from stdin
cat release-notes.md | gh release create v1.2.3 \
  --notes-file -

# Generate GitHub release notes
gh release create v1.2.3 \
  --generate-notes

# Generate notes starting from a specific tag
gh release create v1.2.3 \
  --generate-notes \
  --notes-start-tag v1.2.2

# Use tag annotation / commit message as notes
gh release create v1.2.3 \
  --notes-from-tag

# Draft release
gh release create v1.2.3 \
  --draft

# Prerelease
gh release create v1.2.3 \
  --prerelease

# Mark as latest
gh release create v1.2.3 \
  --latest

# Explicitly not latest
gh release create v1.2.3 \
  --latest=false

# Target branch or commit SHA for tag creation
gh release create v1.2.3 \
  --target main

gh release create v1.2.3 \
  --target abc1234

# Require tag to already exist remotely
gh release create v1.2.3 \
  --verify-tag

# Fail if no commits since previous release
gh release create v1.2.3 \
  --fail-on-no-commits

# Start a discussion
gh release create v1.2.3 \
  --discussion-category "General"

# Upload assets
gh release create v1.2.3 \
  ./dist/*.tgz

# Upload asset with display label
gh release create v1.2.3 \
  './dist/app.zip#Linux x64 build'

# Upload more assets later
gh release upload v1.2.3 ./dist/app.zip

# Replace existing asset with same name
gh release upload v1.2.3 ./dist/app.zip --clobber

# 1. Create as draft
gh release create v1.2.3 ./dist/* \
  --draft \
  --title "v1.2.3" \
  --notes-file release-notes.md

# 2. Verify draft/assets manually or in CI

# 3. Publish
gh release edit v1.2.3 \
  --draft=false


ORG="my-org"
FILE="dependabot.yml"

# Repo with a file
gh search code \
  --owner "$ORG" \
  --filename "$FILE" \
  --json repository,path,url \
  --limit 1000 \
| jq -r '.[] | "\(.repository.nameWithOwner),\(.path),\(.url)"'


ORG="my-org"
FILE="dependabot.yml"

# Bring back the file content as well
gh search code \
  --owner "$ORG" \
  --filename "$FILE" \
  --json repository,path \
  --limit 1000 \
| jq -c '.[]' | while read -r item; do
  repo=$(echo "$item" | jq -r '.repository.nameWithOwner')
  path=$(echo "$item" | jq -r '.path')

  echo "===== $repo:$path ====="

  gh api \
    repos/$repo/contents/$path \
    --jq '.content' \
  | base64 --decode

  echo
done