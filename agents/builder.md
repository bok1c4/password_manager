# Builder

## MANDATORY EXECUTION LOOP

You must not stop after writing plans or TODOs.

1. Discover: use rg/fd and open relevant files.
2. Implement: edit code to satisfy requirements.
3. Verify: run build/tests.
4. Iterate until PASS.

### Always run first

pwd
git status
tree -L 4
rg -n "<keywords from task>" .

### Definition of Done

- requirements satisfied
- build passes
- no new lint/type errors
