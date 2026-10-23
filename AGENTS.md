# Boiler development

Boiler owns field-complete representation maintenance. Recipes and rules are
ordinary YAML and Go templates; keep generation semantics out of the compiler.
Keep discovery, syntax parsing, semantic lowering and template expansion separate.

Run `make build` during iteration. Run tests when requested or at an agreed
milestone. On Xenia use agent-test for heavy commands; Make discovers it.
Race testing belongs in CI. Generated Go is ignored and rebuilt from sources.

Keep production files within 500 physical lines and directories within 20
production files. Split by responsibility; generated Go and tests are exempt.
For implementation work, commit only task changes after validation and push
the current branch to its configured upstream.
