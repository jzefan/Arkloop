-- +goose Up

INSERT INTO skills (org_id, skill_key, version, display_name, description, prompt_md, tool_allowlist, budgets_json, is_active, executor_type, executor_config_json)
VALUES
(
    NULL,
    'lite',
    '1',
    'Lite',
    '基础对话模式，适合简单问答和轻量任务。',
    E'你是一个通用 AI 助手，运行在 Lite 模式下。\n\n保持回复简洁、准确。优先给出直接答案，避免不必要的展开。',
    '{}',
    '{"max_iterations": 5, "max_output_tokens": 2048, "temperature": 0.3}',
    TRUE,
    'agent.simple',
    '{}'
),
(
    NULL,
    'pro',
    '1',
    'Pro',
    '标准工作模式，支持工具调用和多轮推理。',
    E'你是一个通用 AI 助手，运行在 Pro 模式下。\n\n你可以使用工具完成复杂任务。根据用户需求进行多步推理，必要时主动调用工具获取信息或执行操作。保持回复结构化、有深度。',
    '{}',
    '{"max_iterations": 10, "max_output_tokens": 4096}',
    TRUE,
    'agent.simple',
    '{}'
),
(
    NULL,
    'ultra',
    '1',
    'Ultra',
    '高级模式，适合高复杂度任务、长文本生成和深度分析。',
    E'你是一个通用 AI 助手，运行在 Ultra 模式下。\n\n你拥有最高的推理深度和输出能力。面对复杂问题时，进行系统性分析，提供全面、深入的回答。可以处理长文本生成、多步骤推理和跨领域综合分析。必要时主动使用工具。',
    '{}',
    '{"max_iterations": 20, "max_output_tokens": 8192, "temperature": 0.7}',
    TRUE,
    'agent.simple',
    '{}'
),
(
    NULL,
    'auto',
    '1',
    'Auto',
    '自动路由模式，根据任务复杂度分配到 Pro 或 Ultra。',
    '此 skill 使用 classify_route executor，prompt 由 executor_config 内联定义。',
    '{}',
    '{"max_iterations": 1, "max_output_tokens": 8192}',
    TRUE,
    'task.classify_route',
    '{
        "classify_prompt": "分析以下用户消息的任务复杂度。\n如果任务是简单问答、翻译、摘要、格式转换等低复杂度任务，回复 \"pro\"。\n如果任务涉及深度分析、多步推理、代码架构设计、长文本创作等高复杂度任务，回复 \"ultra\"。\n只回复 \"pro\" 或 \"ultra\"，不要输出其他内容。",
        "default_route": "pro",
        "routes": {
            "pro": {
                "prompt_override": "你是一个通用 AI 助手，运行在 Pro 模式下。\n你可以使用工具完成复杂任务。根据用户需求进行多步推理，必要时主动调用工具获取信息或执行操作。保持回复结构化、有深度。"
            },
            "ultra": {
                "prompt_override": "你是一个通用 AI 助手，运行在 Ultra 模式下。\n你拥有最高的推理深度和输出能力。面对复杂问题时，进行系统性分析，提供全面、深入的回答。可以处理长文本生成、多步骤推理和跨领域综合分析。必要时主动使用工具。"
            }
        }
    }'
);

-- +goose Down

DELETE FROM skills WHERE org_id IS NULL AND skill_key IN ('lite', 'pro', 'ultra', 'auto') AND version = '1';
