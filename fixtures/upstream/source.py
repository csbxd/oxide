"""Instrument upstream test attributes without rewriting Rust test bodies.

Only lexical tokens are inspected: comments and literals are opaque, while
attributes inside macro token trees are handled just like ordinary attributes.
Replacing only the identifier preserves source line numbers and comments.
"""

from dataclasses import dataclass


@dataclass(frozen=True)
class _Token:
    value: str
    start: int
    end: int


def _raw_string_end(source: str, start: int) -> int | None:
    for prefix in ("br", "cr", "r"):
        if not source.startswith(prefix, start):
            continue
        cursor = start + len(prefix)
        while cursor < len(source) and source[cursor] == "#":
            cursor += 1
        if cursor == len(source) or source[cursor] != '"':
            continue
        terminator = '"' + "#" * (cursor - start - len(prefix))
        end = source.find(terminator, cursor + 1)
        if end < 0:
            raise ValueError(f"unterminated Rust raw string at offset {start}")
        return end + len(terminator)
    return None


def _string_end(source: str, start: int) -> int:
    cursor = start + 1
    while cursor < len(source):
        if source[cursor] == "\\":
            cursor += 2
        elif source[cursor] == '"':
            return cursor + 1
        else:
            cursor += 1
    raise ValueError(f"unterminated Rust string at offset {start}")


def _character_end(source: str, start: int) -> int | None:
    """Recognize a single character, not an apostrophe starting a lifetime."""
    cursor = start + 1
    if cursor >= len(source):
        return None
    if source[cursor] == "\\":
        cursor += 1
        if source.startswith("u{", cursor):
            end = source.find("}", cursor + 2)
            if end < 0:
                return None
            cursor = end + 1
        elif source.startswith("x", cursor):
            cursor += 3
        else:
            cursor += 1
    else:
        cursor += 1
    if cursor < len(source) and source[cursor] == "'":
        return cursor + 1
    return None


def _tokens(source: str) -> list[_Token]:
    tokens = []
    cursor = 0
    while cursor < len(source):
        start = cursor
        char = source[cursor]
        if char.isspace():
            cursor += 1
            continue
        if source.startswith("//", cursor):
            end = source.find("\n", cursor + 2)
            cursor = len(source) if end < 0 else end + 1
            continue
        if source.startswith("/*", cursor):
            depth = 1
            cursor += 2
            while cursor < len(source) and depth:
                if source.startswith("/*", cursor):
                    depth += 1
                    cursor += 2
                elif source.startswith("*/", cursor):
                    depth -= 1
                    cursor += 2
                else:
                    cursor += 1
            if depth:
                raise ValueError(f"unterminated Rust block comment at offset {start}")
            continue
        raw_end = _raw_string_end(source, cursor) if char in "rbc" else None
        if raw_end is not None:
            cursor = raw_end
            value = "<literal>"
        elif char == '"':
            cursor = _string_end(source, cursor)
            value = "<literal>"
        elif char == "'" and (end := _character_end(source, cursor)) is not None:
            cursor = end
            value = "<literal>"
        elif char.isalpha() or char == "_":
            # A raw identifier is one token; r#test is also a built-in test
            # attribute. This branch runs only after excluding raw strings.
            if source.startswith("r#", cursor):
                cursor += 2
            cursor += 1
            while cursor < len(source) and (
                source[cursor].isalnum() or source[cursor] == "_"
            ):
                cursor += 1
            value = source[start:cursor]
        else:
            cursor += 1
            value = char
        tokens.append(_Token(value, start, cursor))
    return tokens


def _name(token: _Token) -> str:
    return token.value.removeprefix("r#")


def _attribute_end(tokens: list[_Token], opening: int) -> int:
    closing = {"[": "]", "(": ")", "{": "}"}
    stack = ["]"]
    for index in range(opening + 1, len(tokens)):
        value = tokens[index].value
        if value in closing:
            stack.append(closing[value])
        elif value in ("]", ")", "}"):
            if value != stack.pop():
                raise ValueError(f"unbalanced Rust attribute at offset {tokens[opening].start}")
            if not stack:
                return index
    raise ValueError(f"unterminated Rust attribute at offset {tokens[opening].start}")


def _conditional_test(tokens: list[_Token]) -> bool:
    """Recognize test attributes in cfg_attr's attribute arguments only."""
    if len(tokens) == 1:
        return _name(tokens[0]) == "test"
    if (
        len(tokens) < 3
        or _name(tokens[0]) != "cfg_attr"
        or tokens[1].value != "("
        or tokens[-1].value != ")"
    ):
        return False
    arguments = []
    start = 2
    depth = 0
    for index in range(start, len(tokens) - 1):
        value = tokens[index].value
        if value in ("[", "(", "{"):
            depth += 1
        elif value in ("]", ")", "}"):
            depth -= 1
        elif value == "," and depth == 0:
            arguments.append(tokens[start:index])
            start = index + 1
    arguments.append(tokens[start:-1])
    # The first argument is the predicate, where `test` is a valid cfg name.
    return any(_conditional_test(argument) for argument in arguments[1:])


def instrument_tests(source: str) -> str:
    """Replace real #[test] tokens with the fixture's case attribute.

    Conditional test creation is deliberately rejected: the pinned suites do
    not use it, and silently leaving such a test uninstrumented would lose
    coverage. Other cfg_attr attributes, including ignore, remain untouched.
    """
    tokens = _tokens(source)
    replacements = []
    for index, token in enumerate(tokens[:-1]):
        if token.value != "#" or tokens[index + 1].value != "[":
            continue
        end = _attribute_end(tokens, index + 1)
        body = tokens[index + 2 : end]
        if len(body) == 1 and _name(body[0]) == "test":
            replacements.append(body[0])
        elif _conditional_test(body):
            line = source.count("\n", 0, token.start) + 1
            raise ValueError(f"cfg_attr(..., test) is unsupported (line {line})")
    for token in reversed(replacements):
        source = source[: token.start] + "oxide_upstream_macros::case" + source[token.end :]
    return source
