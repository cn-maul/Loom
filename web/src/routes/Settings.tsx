import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Building2 } from 'lucide-react';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { ErrorNote, Notice, Spinner, controlClass } from '../components/ui';
import { aiApi, authToken, configApi } from '../api/client';
import { PageHeader } from '../components/layout';
import type { LLMConfig, PrivacyInfo } from '../api/types';

type Field = { key: keyof LLMConfig; label: string; hint?: string; type?: string; placeholder?: string };

const LLM_FIELDS: Field[] = [
  { key: 'endpoint', label: '对话 API 地址', placeholder: 'https://api.openai.com/v1 或 http://localhost:11434' },
  { key: 'api_key', label: '对话 API Key', hint: '本地 Ollama 留空', type: 'password' },
  { key: 'extract_model', label: '事件提取模型', hint: '记录时抽取摘要、情绪、承诺，建议用快的模型' },
  { key: 'advice_model', label: '建议/周报模型', hint: '负责推理和长文，建议用更强的模型' },
  { key: 'max_tokens', label: '单次补全 token 上限', type: 'number', hint: '太小会让长建议 JSON 被截断' },
];

const EMBED_FIELDS: Field[] = [
  { key: 'embed_endpoint', label: 'Embedding API 地址', hint: '留空则沿用上面的对话 API 地址' },
  { key: 'embed_api_key', label: 'Embedding API Key', hint: '留空则沿用上面的 API Key', type: 'password' },
  { key: 'embed_model', label: 'Embedding 模型', hint: '留空则没有语义检索，只用近期记录兜底' },
  { key: 'embed_dim', label: 'Embedding 维度', type: 'number', hint: '必须与模型输出维度一致；改这里需要重建索引' },
];

const TABS = [
  { id: 'llm', label: '大语言模型' },
  { id: 'embedding', label: '向量模型' },
  { id: 'privacy', label: '隐私与安全' },
  { id: 'other', label: '其他设置' },
] as const;

type Tab = (typeof TABS)[number]['id'];

function Fields({ items, config, patch }: { items: Field[]; config: LLMConfig; patch: (key: keyof LLMConfig, value: string | number) => void }) {
  return (
    <>
      {items.map((item) => (
        <label key={item.key} className="block text-sm text-muted-foreground">
          {item.label}
          <input
            type={item.type ?? 'text'}
            value={String(config[item.key] ?? '')}
            onChange={(e) => patch(item.key, item.type === 'number' ? Number(e.target.value) || 0 : e.target.value)}
            placeholder={item.placeholder}
            className={`${controlClass} mt-1`}
          />
          {item.hint ? <span className="mt-1 block text-xs text-muted-foreground/70">{item.hint}</span> : null}
        </label>
      ))}
    </>
  );
}

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

  const save = async () => {
    if (!config) return;
    setSaving(true);
    setMessage('');
    setNotice('');
    setError('');
    try {
      const result = await configApi.update(config);
      setConfig(result.config.llm);
      setMessage('配置已保存，立即生效');
      if (result.reindex_required) setNotice('Embedding 配置变了，已有向量索引不再匹配，请点「重建向量索引」。');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
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
          tab === 'llm' || tab === 'embedding' ? (
            <Button onClick={save} disabled={saving}>
              {saving ? '保存中…' : '保存配置'}
            </Button>
          ) : null
        }
      />

      {error ? <ErrorNote>{error}</ErrorNote> : null}
      {notice ? <Notice>{notice}</Notice> : null}
      {message ? <p className="text-sm text-green-700 dark:text-green-400">{message}</p> : null}

      <div className="flex gap-6">
        <aside className="w-40 shrink-0 space-y-1">
          {TABS.map((t) => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={`w-full rounded-md px-3 py-2 text-left text-sm transition-colors ${
                tab === t.id ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              }`}
            >
              {t.label}
            </button>
          ))}
        </aside>

        <section className="min-w-0 flex-1 space-y-4">
          {tab === 'llm' ? (
            <div className="space-y-4 rounded-xl border border-border bg-card p-5 shadow-sm">
              <label className="block text-sm text-muted-foreground">
                协议
                <select
                  value={config.protocol}
                  onChange={(e) => patch('protocol', e.target.value)}
                  className={`${controlClass} mt-1 text-foreground`}
                >
                  <option value="openai">OpenAI 兼容</option>
                  <option value="anthropic">Anthropic</option>
                </select>
              </label>
              <Fields items={LLM_FIELDS} config={config} patch={patch} />
            </div>
          ) : null}

          {tab === 'embedding' ? (
            <div className="space-y-4 rounded-xl border border-border bg-card p-5 shadow-sm">
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-semibold text-foreground">Embedding（语义检索）</h2>
                {embedding === null ? (
                  <Badge variant="secondary">向量状态未知</Badge>
                ) : embedding ? (
                  <Badge variant="secondary" className="border-transparent bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-400">
                    向量检索可用
                  </Badge>
                ) : (
                  <Badge variant="outline" className="border-amber-300 bg-amber-50 text-amber-800 dark:border-amber-500/40 dark:bg-amber-950/40 dark:text-amber-300">
                    向量检索不可用
                  </Badge>
                )}
              </div>
              <Fields items={EMBED_FIELDS} config={config} patch={patch} />
              <div className="flex flex-wrap items-center gap-3">
                <Button onClick={checkEmbedding} disabled={checking} variant="outline">
                  {checking ? '检测中…' : '检测 Embedding'}
                </Button>
                <Button onClick={reindex} disabled={reindexing} variant="outline">
                  {reindexing ? '重建中，可能需要几分钟…' : '重建向量索引'}
                </Button>
              </div>
            </div>
          ) : null}

          {tab === 'privacy' && privacy ? (
            <div className="space-y-4">
              <div className="space-y-3 rounded-xl border border-border bg-card p-5 shadow-sm">
                <h2 className="text-sm font-semibold text-foreground">数据去向</h2>
                <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
                  <dt className="text-muted-foreground">服务监听地址</dt>
                  <dd className="font-mono text-foreground">{privacy.listens_on}</dd>
                  <dt className="text-muted-foreground">AI 端点</dt>
                  <dd className="font-mono text-foreground">
                    {privacy.ai_endpoint}{' '}
                    {privacy.ai_endpoint_external ? (
                      <Badge className="ml-1 border-transparent bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300">外部</Badge>
                    ) : (
                      <Badge variant="secondary" className="ml-1">本机</Badge>
                    )}
                  </dd>
                  <dt className="text-muted-foreground">记录原文发送给模型</dt>
                  <dd className="text-foreground">是（提取功能必然包含原文）</dd>
                  <dt className="text-muted-foreground">外部端点开关 (allow_remote)</dt>
                  <dd className="text-foreground">{privacy.allow_remote ? '允许发送到外部端点' : '已关闭：拒绝发送到任何非本机端点'}</dd>
                  <dt className="text-muted-foreground">API 访问控制</dt>
                  <dd className="text-foreground">{privacy.auth_required ? '已开启（需要访问令牌）' : '未开启'}</dd>
                  <dt className="text-muted-foreground">备份加密</dt>
                  <dd className="text-foreground">{privacy.backup_encrypted ? '已开启（AES-256-GCM）' : '未开启（快照为明文）'}</dd>
                </dl>
                <p className="text-xs text-muted-foreground">
                  该面板是事实陈述：改 allow_remote、auth_token、backup.passphrase 请直接编辑 config.yaml 并重启。
                </p>
              </div>

              <div className="space-y-3 rounded-xl border border-border bg-card p-5 shadow-sm">
                <h2 className="text-sm font-semibold text-foreground">本浏览器访问令牌</h2>
                <p className="text-sm text-muted-foreground">
                  {privacy.auth_required
                    ? '服务器已开启访问控制。把令牌填在这里，它只保存在此浏览器的 localStorage，不会发给设置接口。'
                    : '服务器未开启访问控制，无需填写。'}
                </p>
                <div className="flex items-center gap-2">
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
              </div>
            </div>
          ) : null}

          {tab === 'other' ? (
            <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
              <h2 className="mb-2 text-sm font-semibold text-foreground">组织管理</h2>
              <p className="mb-3 text-sm text-muted-foreground">
                组织已移到独立页面：新建、编辑、归档与成员任职都在那里管理。
              </p>
              <Link
                to="/organizations"
                className="inline-flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm text-foreground transition-colors hover:border-primary hover:text-primary"
              >
                <Building2 className="size-4" />
                前往组织管理
              </Link>
            </div>
          ) : null}
        </section>
      </div>
    </div>
  );
}
