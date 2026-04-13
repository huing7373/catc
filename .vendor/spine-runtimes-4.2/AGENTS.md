# Spine Runtimes Agent Rules

## Minimal operating rules
- Keep changes scoped to the requested task.
- Do not commit unless the user explicitly asks.
- Before editing, read files in full, especially if the read tool truncates them.
- Follow existing code style in touched files (naming, type usage, control flow, and error handling patterns).

## Git commit subject prefix (required)
Every commit subject must start with a runtime prefix.

Format:
- Single runtime: `[unity] Fix clipping regression`
- Multiple runtimes: `[c][cpp] Sync physics constraint handling`

Use lowercase prefixes exactly as listed below.

### Runtime prefixes
- `[android]` -> `spine-android`
- `[c]` -> `spine-c`
- `[cocos2dx]` -> `spine-cocos2dx`
- `[cpp]` -> `spine-cpp`
- `[csharp]` -> `spine-csharp`
- `[flutter]` -> `spine-flutter`
- `[glfw]` -> `spine-glfw`
- `[godot]` -> `spine-godot`
- `[haxe]` -> `spine-haxe`
- `[ios]` -> `spine-ios`
- `[libgdx]` -> `spine-libgdx`
- `[monogame]` -> `spine-monogame`
- `[sdl]` -> `spine-sdl`
- `[sfml]` -> `spine-sfml`
- `[ts]` -> `spine-ts`
- `[ue]` -> `spine-ue`
- `[unity]` -> `spine-unity`
- `[xna]` -> `spine-xna`

### Prefix selection rules
- If one runtime is changed, use one prefix.
- If multiple runtimes are changed, include multiple prefixes.
- If shared files at repo root are changed, include the runtime prefix(es) impacted by that change.
