"""Offline schema/reference checks. Needs jsonschema in the verification runtime."""
import copy
import json
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

ROOT = Path(__file__).resolve().parents[2]
CONTRACTS = ROOT / "contracts/futures/v1"
documents = {p.name: json.loads(p.read_text()) for p in CONTRACTS.glob("*.json")}
types = documents["types.schema.json"]
registry = Registry().with_resource(types["$id"], Resource.from_contents(types))


def validator(name):
    return Draft202012Validator(
        {"$ref": types["$id"] + "#/$defs/" + name},
        registry=registry,
        format_checker=FormatChecker(),
    )


def check_refs(node, document):
    if isinstance(node, dict):
        if "$ref" in node:
            ref = node["$ref"]
            file, _, pointer = ref.partition("#")
            target = documents[Path(file).name] if file else document
            for segment in pointer.lstrip("/").split("/") if pointer else []:
                target = target[segment.replace("~1", "/").replace("~0", "~")]
        for value in node.values():
            check_refs(value, document)
    elif isinstance(node, list):
        for item in node:
            check_refs(item, document)


for document in documents.values():
    check_refs(document, document)
Draft202012Validator.check_schema(types)
for name, definition in types["$defs"].items():
    Draft202012Validator.check_schema(definition)

for path in (CONTRACTS / "fixtures").glob("*.json"):
    name = path.name.split("-")[0].title()
    validator(name).validate(json.loads(path.read_text()))

task = json.loads((CONTRACTS / "fixtures/task-offline.json").read_text())
for mutate in (
    lambda v: v["scope"].update(owner_id=0),
    lambda v: v["scope"].update(mode="demo"),
    lambda v: v.update(secret="must-not-be-in-task"),
    lambda v: v.update(allowed_tools=["get_financials"]),
    lambda v: v["limits"].update(max_model_calls=9),
    lambda v: v.update(as_of="not-a-timestamp"),
):
    invalid = copy.deepcopy(task)
    mutate(invalid)
    assert list(validator("Task").iter_errors(invalid)), invalid

record = json.loads((CONTRACTS / "fixtures/record-offline.json").read_text())
for mutate in (
    lambda v: v.update(scope="private", owner_id=None),
    lambda v: v.update(value=1000.0),
    lambda v: v["quality"].update(acquisition="fetch_failed"),
    lambda v: v.update(source_type="fixture"),
):
    invalid = copy.deepcopy(record)
    mutate(invalid)
    assert list(validator("Record").iter_errors(invalid)), invalid

print("PASS: local refs, schema definitions, 5 fixtures, 10 invalid scope/data cases")
print("NOTE: this does not verify business authorization, data rights or live readiness")
