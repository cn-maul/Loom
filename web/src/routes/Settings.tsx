import { useEffect, useState } from 'react';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { ErrorNote, Notice, Spinner, controlClass } from '../components/ui';
import { aiApi, authToken, configApi } from '../api/client';
import { Field, PageHeader, SectionCard } from '../components/layout';
import { cn } from '../lib/utils';
import type { LLMConfig, PrivacyInfo } from '../api/types';

const TABS = [
  { id: 'llm', label: '大语言模型' },
  { id: 'embedding', label: '向量模型' },
  { id: 'rerank', label: '重排模型' },
  { id: 'privacy', label: '隐私与安全' },
] as const;

type Tab = (typeof TABS)[number]['id'];

export default function Settings() {
  const [config, setConfig] = useState<LLMConfig | null>(null);
  const [privacy, setPrivacy] = useState<PrivacyInfo | null>(null);
  const [tab, setTab] = useState<Tab>('llm');
  const [saving, setSaving] = useState(false);
  const [reindexing, setReindexing] = useState(false);
  const [checking, setChecking] = useState(false);
  const [message, setMessage] = useState('');
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [embedding, setEmbedding] = useState<boolean | null>(null);
  const [tokenDraft, setTokenDraft] = useState('');
  const [modelIds, setModelIds] = useState<string[]>([]);
  const [fetchingModels, setFetchingModels] = useState(false);

  useEffect(() => {
    configApi
      .get()
      .then((data) => {
        setConfig(data.config.llm);
        setPrivacy(data.privacy);
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
    aiApi
      .embeddingStatus()
      .then((data) => setEmbedding(data.available))
      .catch(() => setEmbedding(false));
  }, []);

  const patch = (key: keyof LLMConfig, value: string | number) => {
    setConfig((current) => (current ? ({ ...current, [key]: value } as LLMConfig) : current));
  };

  const save = async (): Promise<boolean> => {
    if (!config) return false;
    setSaving(true);
    setMessage('');
    setNotice('');
    setError('');
    try {
      const result = await configApi.update(config);
      setConfig(result.config.llm);
      setMessage('配置已保存，立即生效');
      if (result.reindex_required) setNotice('Embedding 配置变了，已有向量索引不再匹配，请点「重建向量索引」。');
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setSaving(false);
    }
  };

  // 先落库再探测：后端按刚保存的端点/Key 去请求 /models，拿到目录后
  // 两个模型输入框出现下拉建议，但仍然允许手输任意模型 id。
  const fetchModels = async () => {
    setFetchingModels(true);
    setMessage('');
    setError('');
    try {
      if (!(await save())) return;
      const ids = await aiApi.models();
      setModelIds(ids);
      setMessage(
        ids.length > 0
          ? `获取到 ${ids.length} 个模型：点击模型输入框可从下拉选择，也可以直接输入`
          : '端点返回了空模型列表，仍可直接输入模型 id',
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setFetchingModels(false);
    }
  };

  const checkEmbedding = async () => {
    setChecking(true);
    setError('');
    try {
      const data = await aiApi.embeddingStatus();
      setEmbedding(data.available);
      setMessage(data.available ? 'Embedding 服务可用' : 'Embedding 服务不可用，请检查地址、Key、模型名');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setChecking(false);
    }
  };

  const reindex = async () => {
    if (!window.confirm('重建会清空现有向量索引并重新计算每条记录和画像的向量，可能耗时数分钟。继续？')) return;
    setReindexing(true);
    setError('');
    setMessage('');
    try {
      const result = await aiApi.reindex();
      setMessage(`索引完成：事件 ${result.events_indexed} 条，画像 ${result.traits_indexed} 条${result.failed ? `，失败 ${result.failed} 条` : ''}`);
      setNotice('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setReindexing(false);
    }
  };

  if (error && !config) {
    return <ErrorNote>{error}</ErrorNote>;
  }

  if (!config) {
    return <Spinner label="载入配置…" />;
  }

  return (
    <div>
      <PageHeader
        title="设置"
        description="模型端点、协议与密钥；改完记得保存"
        actions={
          tab === 'llm' || tab === 'embedding' || tab === 'rerank' ? (
            <Button onClick={save} disabled={saving}>
              {saving ? '保存中…' : '保存配置'}
            </Button>
          ) : null
        }
      />

      {error ? (
        <div className="mb-5">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}
      {notice ? (
        <div className="mb-5">
          <Notice>{notice}</Notice>
        </div>
      ) : null}
      {message ? <p className="mb-5 text-[13px] leading-[1.7] text-live-text">{message}</p> : null}

      <div className="flex gap-6">
        {/* Wayfinding: the sections are a list, so the rail is a list of rows
            on a quiet fill — no coloured plate, no left bar. */}
        <aside className="w-40 shrink-0">
          <nav className="space-y-0.5">
            {TABS.map((t) => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className={cn(
                  'flex h-9 w-full items-center rounded-md px-3 text-left text-[13.5px] transition-colors',
                  tab === t.id
                    ? 'bg-fill font-semibold text-foreground'
                    : 'text-ink-2 hover:bg-fill/60 hover:text-foreground',
                )}
              >
                {t.label}
              </button>
            ))}
          </nav>
        </aside>

        <section className="min-w-0 flex-1 space-y-9">
          {tab === 'llm' ? (
            <section className="panel space-y-3.5 p-5">
              {/* 第一行：协议（窄下拉）+ 对话 API 地址 */}
              <div className="grid grid-cols-[9rem_minmax(0,1fr)] gap-3.5">
                <Field label="协议">
                  <select value={config.protocol} onChange={(e) => patch('protocol', e.target.value)} className={controlClass}>
                    <option value="openai">OpenAI 兼容</option>
                    <option value="anthropic">Anthropic</option>
                  </select>
                </Field>
                <Field label="对话 API 地址" hint="本地 Ollama / LM Studio 填本机地址">
                  <input
                    value={String(config.endpoint ?? '')}
                    onChange={(e) => patch('endpoint', e.target.value)}
                    placeholder="https://api.openai.com/v1 或 http://localhost:11434"
                    className={controlClass}
                  />
                </Field>
              </div>

              {/* 第二行：API Key + 补全 token 上限 */}
              <div className="grid grid-cols-[minmax(0,1fr)_11rem] gap-3.5">
                <Field label="对话 API Key" hint="本地 Ollama 留空">
                  <input
                    type="password"
                    value={String(config.api_key ?? '')}
                    onChange={(e) => patch('api_key', e.target.value)}
                    className={controlClass}
                  />
                </Field>
                <Field label="单次补全 token 上限" hint="太小会截断长建议 JSON">
                  <input
                    type="number"
                    value={String(config.max_tokens ?? '')}
                    onChange={(e) => patch('max_tokens', Number(e.target.value) || 0)}
                    className={controlClass}
                  />
                </Field>
              </div>

              {/* 第三行：两个模型 */}
              <div className="grid gap-3.5 sm:grid-cols-2">
                <Field label="事件提取模型" hint="抽取摘要、情绪、承诺，建议用快的模型">
                  <input
                    list="llm-model-list"
                    value={String(config.extract_model ?? '')}
                    onChange={(e) => patch('extract_model', e.target.value)}
                    placeholder="输入或从下拉选择"
                    className={controlClass}
                  />
                </Field>
                <Field label="建议/周报模型" hint="负责推理和长文，建议用更强的模型">
                  <input
                    list="llm-model-list"
                    value={String(config.advice_model ?? '')}
                    onChange={(e) => patch('advice_model', e.target.value)}
                    placeholder="输入或从下拉选择"
                    className={controlClass}
                  />
                </Field>
              </div>

              <div>
                <Button variant="outline" onClick={() => void fetchModels()} disabled={fetchingModels}>
                  {fetchingModels ? '获取中…' : '获取模型'}
                </Button>
              </div>

              <datalist id="llm-model-list">
                {modelIds.map((id) => (
                  <option key={id} value={id} />
                ))}
              </datalist>
            </section>
          ) : null}

          {tab === 'embedding' ? (
            <section className="panel space-y-3.5 p-5">
              <div className="flex items-center justify-between">
                <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">Embedding（语义检索）</h2>
                {embedding === null ? (
                  <Badge variant="secondary">向量状态未知</Badge>
                ) : embedding ? (
                  <Badge className="bg-live-bg text-live-text">向量检索可用</Badge>
                ) : (
                  <Badge className="bg-heat-bg text-heat-text">向量检索不可用</Badge>
                )}
              </div>

              {/* 第一行：Embedding API 地址 */}
              <Field label="Embedding API 地址" hint="语义检索的独立端点；本地 Ollama / LM Studio 填本机地址">
                <input
                  value={String(config.embed_endpoint ?? '')}
                  onChange={(e) => patch('embed_endpoint', e.target.value)}
                  placeholder="留空则沿用对话 API 地址，如 http://localhost:11434"
                  className={controlClass}
                />
              </Field>

              {/* 第二行：Embedding API Key + 维度 */}
              <div className="grid grid-cols-[minmax(0,1fr)_11rem] gap-3.5">
                <Field label="Embedding API Key" hint="留空则沿用对话 API Key">
                  <input
                    type="password"
                    value={String(config.embed_api_key ?? '')}
                    onChange={(e) => patch('embed_api_key', e.target.value)}
                    className={controlClass}
                  />
                </Field>
                <Field label="Embedding 维度" hint="改这里需要重建索引">
                  <input
                    type="number"
                    value={String(config.embed_dim ?? '')}
                    onChange={(e) => patch('embed_dim', Number(e.target.value) || 0)}
                    className={controlClass}
                  />
                </Field>
              </div>

              {/* 第三行：Embedding 模型 */}
              <Field label="Embedding 模型" hint="留空则没有语义检索，只用近期记录兜底">
                <input
                  value={String(config.embed_model ?? '')}
                  onChange={(e) => patch('embed_model', e.target.value)}
                  placeholder="如 nomic-embed-text"
                  className={controlClass}
                />
              </Field>

              <div className="flex flex-wrap gap-2">
                <Button variant="outline" onClick={checkEmbedding} disabled={checking}>
                  {checking ? '检测中…' : '检测 Embedding'}
                </Button>
                <Button variant="outline" onClick={reindex} disabled={reindexing}>
                  {reindexing ? '重建中，可能需要几分钟…' : '重建向量索引'}
                </Button>
              </div>
            </section>
          ) : null}

          {tab === 'rerank' ? (
            <section className="panel space-y-3.5 p-5">
              <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">Rerank（检索重排）</h2>

              {/* 第一行：Rerank API 地址 */}
              <Field
                label="Rerank API 地址"
                hint="重排与向量模型不一家时（Jina、Cohere 等）在这里单独填，Cohere 格式 /rerank 接口"
              >
                <input
                  value={String(config.rerank_endpoint ?? '')}
                  onChange={(e) => patch('rerank_endpoint', e.target.value)}
                  placeholder="留空则沿用 Embedding API 地址"
                  className={controlClass}
                />
              </Field>

              {/* 第二行：Rerank API Key */}
              <Field label="Rerank API Key" hint="留空则沿用 Embedding API Key">
                <input
                  type="password"
                  value={String(config.rerank_api_key ?? '')}
                  onChange={(e) => patch('rerank_api_key', e.target.value)}
                  className={controlClass}
                />
              </Field>

              {/* 第三行：Rerank 模型 */}
              <Field label="Rerank 模型" hint="配置后，建议生成的检索证据先经重排再进提示词；检索时失败会如实报告，不静默降级">
                <input
                  value={String(config.rerank_model ?? '')}
                  onChange={(e) => patch('rerank_model', e.target.value)}
                  placeholder="留空关闭，如 bge-reranker-v2-m3"
                  className={controlClass}
                />
              </Field>
            </section>
          ) : null}

          {tab === 'privacy' && privacy ? (
            <div className="space-y-9">
              <SectionCard title="数据去向">
                <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2.5">
                  <dt className="text-[13px] text-ink-3">服务监听地址</dt>
                  <dd className="font-mono text-[13px] text-foreground">{privacy.listens_on}</dd>
                  <dt className="text-[13px] text-ink-3">AI 端点</dt>
                  <dd className="flex flex-wrap items-center gap-1.5 font-mono text-[13px] text-foreground">
                    {privacy.ai_endpoint}
                    {privacy.ai_endpoint_external ? (
                      <Badge className="bg-heat-bg font-sans text-heat-text">外部</Badge>
                    ) : (
                      <Badge variant="secondary" className="font-sans">
                        本机
                      </Badge>
                    )}
                  </dd>
                  <dt className="text-[13px] text-ink-3">记录原文发送给模型</dt>
                  <dd className="text-[13px] text-foreground">是（提取功能必然包含原文）</dd>
                  <dt className="text-[13px] text-ink-3">外部端点开关 (allow_remote)</dt>
                  <dd className="text-[13px] text-foreground">
                    {privacy.allow_remote ? '允许发送到外部端点' : '已关闭：拒绝发送到任何非本机端点'}
                  </dd>
                  <dt className="text-[13px] text-ink-3">API 访问控制</dt>
                  <dd className="text-[13px] text-foreground">
                    {privacy.auth_required ? '已开启（需要访问令牌）' : '未开启'}
                  </dd>
                  <dt className="text-[13px] text-ink-3">备份加密</dt>
                  <dd className="text-[13px] text-foreground">
                    {privacy.backup_encrypted ? '已开启（AES-256-GCM）' : '未开启（快照为明文）'}
                  </dd>
                </dl>
              </SectionCard>

              <SectionCard title="本浏览器访问令牌">
                <p className="text-[13.5px] leading-[1.7] text-muted-foreground">
                  {privacy.auth_required
                    ? '服务器已开启访问控制。把令牌填在这里，它只保存在此浏览器的 localStorage，不会发给设置接口。'
                    : '服务器未开启访问控制，无需填写。'}
                </p>
                <div className="mt-3.5 flex flex-wrap items-center gap-2">
                  <input
                    type="password"
                    value={tokenDraft}
                    onChange={(e) => setTokenDraft(e.target.value)}
                    placeholder={authToken.get() ? '已保存令牌，可输入新值覆盖' : '访问令牌'}
                    className={`${controlClass} max-w-xs`}
                  />
                  <Button
                    variant="outline"
                    onClick={() => {
                      authToken.set(tokenDraft.trim());
                      setTokenDraft('');
                      setMessage(tokenDraft.trim() ? '令牌已保存到本浏览器' : '令牌已清除');
                    }}
                    disabled={!tokenDraft.trim() && !authToken.get()}
                  >
                    保存
                  </Button>
                  {authToken.get() ? (
                    <Button
                      variant="ghost"
                      onClick={() => {
                        authToken.set('');
                        setTokenDraft('');
                        setMessage('令牌已清除');
                      }}
                    >
                      清除
                    </Button>
                  ) : null}
                </div>
                <p className="mt-3.5 text-[12px] leading-[1.6] text-ink-3">
                  该面板是事实陈述：改 allow_remote、auth_token、backup.passphrase 请直接编辑 config.yaml 并重启。
                </p>
              </SectionCard>
            </div>
          ) : null}
        </section>
      </div>
    </div>
  );
}
