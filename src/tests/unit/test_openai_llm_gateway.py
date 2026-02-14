from __future__ import annotations

import json

import anyio
import httpx

from packages.llm_gateway import (
    ERROR_CLASS_PROVIDER_NON_RETRYABLE,
    LlmGatewayRequest,
    LlmMessage,
    LlmStreamLlmRequest,
    LlmStreamLlmResponseChunk,
    LlmStreamMessageDelta,
    LlmStreamProviderFallback,
    LlmStreamRunCompleted,
    LlmStreamRunFailed,
    LlmStreamToolCall,
    LlmTextPart,
    ToolSpec,
)
from packages.llm_gateway.openai import OpenAiGatewayConfig, OpenAiLlmGateway


def test_openai_gateway_chat_completions_streams_deltas_and_completed() -> None:
    sse = (
        'data: {"choices":[{"delta":{"role":"assistant","content":"hello"}}]}\n\n'
        'data: {"choices":[{"delta":{"content":" world"}}]}\n\n'
        "data: [DONE]\n\n"
    )

    def _handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/chat/completions"
        assert request.headers.get("authorization") == "Bearer sk-test"

        payload = json.loads(request.content)
        assert payload["model"] == "gpt-test"
        assert payload["stream"] is True
        assert payload["messages"] == [{"role": "user", "content": "hi"}]

        return httpx.Response(
            200,
            headers={"content-type": "text/event-stream"},
            content=sse.encode("utf-8"),
        )

    transport = httpx.MockTransport(_handler)
    client = httpx.AsyncClient(base_url="https://example.test/v1", transport=transport)
    gateway = OpenAiLlmGateway(
        config=OpenAiGatewayConfig(
            api_key="sk-test",
            base_url="https://example.test/v1",
            api_mode="chat_completions",
            total_timeout_seconds=5.0,
        ),
        client=client,
    )

    request = LlmGatewayRequest(
        model="gpt-test",
        messages=[LlmMessage(role="user", content=[LlmTextPart(text="hi")])],
    )

    async def _collect() -> list[object]:
        items: list[object] = []
        async for item in gateway.stream(request=request):
            items.append(item)
        await client.aclose()
        return items

    items = anyio.run(_collect)

    assert [type(item) for item in items] == [
        LlmStreamMessageDelta,
        LlmStreamMessageDelta,
        LlmStreamRunCompleted,
    ]
    assert items[0].content_delta == "hello"
    assert items[1].content_delta == " world"


def test_openai_gateway_chat_completions_emits_tool_call_events_from_stream() -> None:
    chunk1 = {
        "choices": [
            {
                "delta": {
                    "tool_calls": [
                        {
                            "index": 0,
                            "id": "call_1",
                            "type": "function",
                            "function": {"name": "echo", "arguments": '{"text":"hi"'},
                        }
                    ]
                }
            }
        ]
    }
    chunk2 = {
        "choices": [
            {
                "delta": {"tool_calls": [{"index": 0, "function": {"arguments": "}"}}]},
                "finish_reason": "tool_calls",
            }
        ]
    }
    sse = f"data: {json.dumps(chunk1)}\n\n" f"data: {json.dumps(chunk2)}\n\n" "data: [DONE]\n\n"

    def _handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/chat/completions"
        return httpx.Response(
            200,
            headers={"content-type": "text/event-stream"},
            content=sse.encode("utf-8"),
        )

    transport = httpx.MockTransport(_handler)
    client = httpx.AsyncClient(base_url="https://example.test/v1", transport=transport)
    gateway = OpenAiLlmGateway(
        config=OpenAiGatewayConfig(
            api_key="sk-test",
            base_url="https://example.test/v1",
            api_mode="chat_completions",
            total_timeout_seconds=5.0,
        ),
        client=client,
    )

    request = LlmGatewayRequest(
        model="gpt-test",
        messages=[LlmMessage(role="user", content=[LlmTextPart(text="hi")])],
    )

    async def _collect() -> list[object]:
        items: list[object] = []
        async for item in gateway.stream(request=request):
            items.append(item)
        await client.aclose()
        return items

    items = anyio.run(_collect)

    assert [type(item) for item in items] == [
        LlmStreamToolCall,
        LlmStreamRunCompleted,
    ]
    assert items[0].tool_call_id == "call_1"
    assert items[0].tool_name == "echo"
    assert items[0].arguments_json == {"text": "hi"}


def test_openai_gateway_chat_completions_sends_tools_and_tool_results() -> None:
    sse = "data: [DONE]\n\n"

    def _handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/chat/completions"

        payload = json.loads(request.content)
        assert payload["tools"] == [
            {
                "type": "function",
                "function": {
                    "name": "echo",
                    "description": "echo tool",
                    "parameters": {"type": "object"},
                },
            }
        ]
        assert payload["tool_choice"] == "auto"
        assert payload["messages"] == [
            {"role": "user", "content": "hi"},
            {
                "role": "assistant",
                "content": "",
                "tool_calls": [
                    {
                        "id": "call_1",
                        "type": "function",
                        "function": {"name": "echo", "arguments": '{"text":"hi"}'},
                    }
                ],
            },
            {"role": "tool", "tool_call_id": "call_1", "content": '{"text":"ok"}'},
        ]

        return httpx.Response(
            200,
            headers={"content-type": "text/event-stream"},
            content=sse.encode("utf-8"),
        )

    transport = httpx.MockTransport(_handler)
    client = httpx.AsyncClient(base_url="https://example.test/v1", transport=transport)
    gateway = OpenAiLlmGateway(
        config=OpenAiGatewayConfig(
            api_key="sk-test",
            base_url="https://example.test/v1",
            api_mode="chat_completions",
            total_timeout_seconds=5.0,
        ),
        client=client,
    )

    request = LlmGatewayRequest(
        model="gpt-test",
        messages=[
            LlmMessage(role="user", content=[LlmTextPart(text="hi")]),
            LlmMessage(
                role="assistant",
                content=[],
                tool_calls=[
                    LlmStreamToolCall(
                        tool_call_id="call_1",
                        tool_name="echo",
                        arguments_json={"text": "hi"},
                    )
                ],
            ),
            LlmMessage(
                role="tool",
                content=[
                    LlmTextPart(
                        text=json.dumps(
                            {
                                "tool_call_id": "call_1",
                                "tool_name": "echo",
                                "result": {"text": "ok"},
                            },
                            ensure_ascii=False,
                        )
                    )
                ],
            ),
        ],
        tools=[ToolSpec(name="echo", description="echo tool", json_schema={"type": "object"})],
    )

    async def _collect() -> list[object]:
        items: list[object] = []
        async for item in gateway.stream(request=request):
            items.append(item)
        await client.aclose()
        return items

    items = anyio.run(_collect)
    assert [type(item) for item in items] == [LlmStreamRunCompleted]


def test_openai_gateway_responses_sends_tools_and_tool_results() -> None:
    sse = 'data: {"type":"response.completed","response":{}}\n\n'

    def _handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/responses"

        payload = json.loads(request.content)
        assert payload["tools"] == [
            {
                "type": "function",
                "name": "echo",
                "description": "echo tool",
                "parameters": {"type": "object"},
            }
        ]
        assert payload["tool_choice"] == "auto"
        assert payload["input"] == [
            {"role": "user", "content": [{"type": "input_text", "text": "hi"}]},
            {
                "type": "function_call",
                "call_id": "call_1",
                "name": "echo",
                "arguments": '{"text":"hi"}',
            },
            {"type": "function_call_output", "call_id": "call_1", "output": '{"text":"ok"}'},
        ]

        return httpx.Response(
            200,
            headers={"content-type": "text/event-stream"},
            content=sse.encode("utf-8"),
        )

    transport = httpx.MockTransport(_handler)
    client = httpx.AsyncClient(base_url="https://example.test/v1", transport=transport)
    gateway = OpenAiLlmGateway(
        config=OpenAiGatewayConfig(
            api_key="sk-test",
            base_url="https://example.test/v1",
            api_mode="responses",
            total_timeout_seconds=5.0,
        ),
        client=client,
    )

    request = LlmGatewayRequest(
        model="gpt-test",
        messages=[
            LlmMessage(role="user", content=[LlmTextPart(text="hi")]),
            LlmMessage(
                role="assistant",
                content=[],
                tool_calls=[
                    LlmStreamToolCall(
                        tool_call_id="call_1",
                        tool_name="echo",
                        arguments_json={"text": "hi"},
                    )
                ],
            ),
            LlmMessage(
                role="tool",
                content=[
                    LlmTextPart(
                        text=json.dumps(
                            {
                                "tool_call_id": "call_1",
                                "tool_name": "echo",
                                "result": {"text": "ok"},
                            },
                            ensure_ascii=False,
                        )
                    )
                ],
            ),
        ],
        tools=[ToolSpec(name="echo", description="echo tool", json_schema={"type": "object"})],
    )

    async def _collect() -> list[object]:
        items: list[object] = []
        async for item in gateway.stream(request=request):
            items.append(item)
        await client.aclose()
        return items

    items = anyio.run(_collect)
    assert [type(item) for item in items] == [LlmStreamRunCompleted]


def test_openai_gateway_chat_completions_invalid_tool_call_emits_run_failed() -> None:
    chunk = {
        "choices": [
            {
                "delta": {
                    "tool_calls": [
                        {
                            "index": 0,
                            "type": "function",
                            "function": {"name": "echo", "arguments": '{"text":"hi"}'},
                        }
                    ]
                },
                "finish_reason": "tool_calls",
            }
        ]
    }
    sse = f"data: {json.dumps(chunk)}\n\n" "data: [DONE]\n\n"

    def _handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/chat/completions"
        return httpx.Response(
            200,
            headers={"content-type": "text/event-stream"},
            content=sse.encode("utf-8"),
        )

    transport = httpx.MockTransport(_handler)
    client = httpx.AsyncClient(base_url="https://example.test/v1", transport=transport)
    gateway = OpenAiLlmGateway(
        config=OpenAiGatewayConfig(
            api_key="sk-test",
            base_url="https://example.test/v1",
            api_mode="chat_completions",
            total_timeout_seconds=5.0,
        ),
        client=client,
    )

    request = LlmGatewayRequest(
        model="gpt-test",
        messages=[LlmMessage(role="user", content=[LlmTextPart(text="hi")])],
    )

    async def _collect() -> list[object]:
        items: list[object] = []
        async for item in gateway.stream(request=request):
            items.append(item)
        await client.aclose()
        return items

    items = anyio.run(_collect)

    assert [type(item) for item in items] == [LlmStreamRunFailed]
    assert items[0].error.error_class == ERROR_CLASS_PROVIDER_NON_RETRYABLE
    assert "tool_calls[0]" in str(items[0].error.details.get("reason"))


def test_openai_gateway_responses_streams_deltas_and_completed_with_usage() -> None:
    sse = (
        'data: {"type":"response.output_text.delta","delta":"hello"}\n\n'
        'data: {"type":"response.output_text.delta","delta":" world"}\n\n'
        'data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}\n\n'
    )

    def _handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/responses"
        assert request.headers.get("authorization") == "Bearer sk-test"

        payload = json.loads(request.content)
        assert payload["model"] == "gpt-test"
        assert payload["stream"] is True
        assert payload["input"] == [{"role": "user", "content": [{"type": "input_text", "text": "hi"}]}]

        return httpx.Response(
            200,
            headers={"content-type": "text/event-stream"},
            content=sse.encode("utf-8"),
        )

    transport = httpx.MockTransport(_handler)
    client = httpx.AsyncClient(base_url="https://example.test/v1", transport=transport)
    gateway = OpenAiLlmGateway(
        config=OpenAiGatewayConfig(
            api_key="sk-test",
            base_url="https://example.test/v1",
            api_mode="responses",
            total_timeout_seconds=5.0,
        ),
        client=client,
    )

    request = LlmGatewayRequest(
        model="gpt-test",
        messages=[LlmMessage(role="user", content=[LlmTextPart(text="hi")])],
    )

    async def _collect() -> list[object]:
        items: list[object] = []
        async for item in gateway.stream(request=request):
            items.append(item)
        await client.aclose()
        return items

    items = anyio.run(_collect)

    assert [type(item) for item in items] == [
        LlmStreamMessageDelta,
        LlmStreamMessageDelta,
        LlmStreamRunCompleted,
    ]
    assert items[0].content_delta == "hello"
    assert items[1].content_delta == " world"
    assert items[2].usage is not None
    assert items[2].usage.total_tokens == 3


def test_openai_gateway_responses_emits_tool_call_events_from_output() -> None:
    sse = (
        'data: {"type":"response.completed","response":{"output":[{"type":"function_call","id":"call_1","name":"echo","arguments":"{\\"text\\":\\"hi\\"}"}]}}\n\n'
    )

    def _handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/responses"
        assert request.headers.get("authorization") == "Bearer sk-test"
        return httpx.Response(
            200,
            headers={"content-type": "text/event-stream"},
            content=sse.encode("utf-8"),
        )

    transport = httpx.MockTransport(_handler)
    client = httpx.AsyncClient(base_url="https://example.test/v1", transport=transport)
    gateway = OpenAiLlmGateway(
        config=OpenAiGatewayConfig(
            api_key="sk-test",
            base_url="https://example.test/v1",
            api_mode="responses",
            total_timeout_seconds=5.0,
        ),
        client=client,
    )

    request = LlmGatewayRequest(
        model="gpt-test",
        messages=[LlmMessage(role="user", content=[LlmTextPart(text="hi")])],
    )

    async def _collect() -> list[object]:
        items: list[object] = []
        async for item in gateway.stream(request=request):
            items.append(item)
        await client.aclose()
        return items

    items = anyio.run(_collect)

    assert [type(item) for item in items] == [
        LlmStreamToolCall,
        LlmStreamRunCompleted,
    ]
    assert items[0].tool_call_id == "call_1"
    assert items[0].tool_name == "echo"
    assert items[0].arguments_json == {"text": "hi"}


def test_openai_gateway_auto_falls_back_from_responses_to_chat_completions() -> None:
    sse = 'data: {"choices":[{"delta":{"content":"ok"}}]}\n\n' "data: [DONE]\n\n"
    call_paths: list[str] = []

    def _handler(request: httpx.Request) -> httpx.Response:
        call_paths.append(request.url.path)
        if request.url.path == "/v1/responses":
            return httpx.Response(
                404,
                json={"error": {"message": "Not Found", "type": "not_found"}},
            )
        if request.url.path == "/v1/chat/completions":
            return httpx.Response(
                200,
                headers={"content-type": "text/event-stream"},
                content=sse.encode("utf-8"),
            )
        raise AssertionError(f"unexpected path: {request.url.path}")

    transport = httpx.MockTransport(_handler)
    client = httpx.AsyncClient(base_url="https://example.test/v1", transport=transport)
    gateway = OpenAiLlmGateway(
        config=OpenAiGatewayConfig(
            api_key="sk-test",
            base_url="https://example.test/v1",
            api_mode="auto",
            total_timeout_seconds=5.0,
        ),
        client=client,
    )

    request = LlmGatewayRequest(
        model="gpt-test",
        messages=[LlmMessage(role="user", content=[LlmTextPart(text="hi")])],
    )

    async def _collect() -> list[object]:
        items: list[object] = []
        async for item in gateway.stream(request=request):
            items.append(item)
        await client.aclose()
        return items

    items = anyio.run(_collect)

    assert call_paths == ["/v1/responses", "/v1/chat/completions"]
    assert [type(item) for item in items] == [
        LlmStreamProviderFallback,
        LlmStreamMessageDelta,
        LlmStreamRunCompleted,
    ]
    assert items[0].from_api_mode == "responses"
    assert items[0].to_api_mode == "chat_completions"
    assert items[0].status_code == 404


def test_openai_gateway_emits_llm_debug_events_when_enabled() -> None:
    sse = (
        'data: {"choices":[{"delta":{"role":"assistant","content":"hello"}}]}\n\n'
        'data: {"choices":[{"delta":{"content":" world"}}]}\n\n'
        "data: [DONE]\n\n"
    )

    def _handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/chat/completions"
        return httpx.Response(
            200,
            headers={"content-type": "text/event-stream"},
            content=sse.encode("utf-8"),
        )

    transport = httpx.MockTransport(_handler)
    client = httpx.AsyncClient(base_url="https://example.test/v1", transport=transport)
    gateway = OpenAiLlmGateway(
        config=OpenAiGatewayConfig(
            api_key="sk-test",
            base_url="https://example.test/v1",
            api_mode="chat_completions",
            total_timeout_seconds=5.0,
            emit_llm_debug_events=True,
        ),
        client=client,
    )

    request = LlmGatewayRequest(
        model="gpt-test",
        messages=[LlmMessage(role="user", content=[LlmTextPart(text="hi")])],
    )

    async def _collect() -> list[object]:
        items: list[object] = []
        async for item in gateway.stream(request=request):
            items.append(item)
        await client.aclose()
        return items

    items = anyio.run(_collect)

    assert [type(item) for item in items] == [
        LlmStreamLlmRequest,
        LlmStreamLlmResponseChunk,
        LlmStreamMessageDelta,
        LlmStreamLlmResponseChunk,
        LlmStreamMessageDelta,
        LlmStreamLlmResponseChunk,
        LlmStreamRunCompleted,
    ]
    assert items[0].payload_json["model"] == "gpt-test"
    assert items[1].raw.startswith('{"choices"')
    assert items[2].content_delta == "hello"
    assert items[4].content_delta == " world"
    assert items[5].raw == "[DONE]"
    assert {item.llm_call_id for item in items if hasattr(item, "llm_call_id")} == {items[0].llm_call_id}
