import { useState, useEffect } from 'react';
import { configApi } from '../api/client';
import type { LLMConfig } from '../api/types';

export default function Settings() {
  const [config, setConfig] = useState<LLMConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');

  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    try {
      const data = await configApi.get();
      setConfig(data.llm);
    } catch (error) {
      console.error('Failed to load config:', error);
      setMessage('加载配置失败');
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    if (!config) return;

    setSaving(true);
    setMessage('');
    try {
      await configApi.update(config);
      setMessage('配置已保存');
    } catch (error) {
      console.error('Failed to save config:', error);
      setMessage('保存配置失败');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <div className="p-4">加载中...</div>;
  }

  if (!config) {
    return <div className="p-4">无法加载配置</div>;
  }

  return (
    <div className="max-w-2xl mx-auto p-6">
      <h1 className="text-2xl font-bold mb-6">LLM 配置</h1>

      <div className="space-y-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            API 地址
          </label>
          <input
            type="text"
            value={config.endpoint}
            onChange={(e) => setConfig({ ...config, endpoint: e.target.value })}
            placeholder="http://localhost:11434"
            className="w-full px-3 py-2 border rounded-lg"
          />
          <p className="text-sm text-gray-500 mt-1">
            Ollama: http://localhost:11434 / DeepSeek: https://api.deepseek.com
          </p>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            协议
          </label>
          <select
            value={config.protocol}
            onChange={(e) => setConfig({ ...config, protocol: e.target.value })}
            className="w-full px-3 py-2 border rounded-lg"
          >
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic</option>
          </select>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            API Key（可选，本地Ollama不需要）
          </label>
          <input
            type="password"
            value={config.api_key}
            onChange={(e) => setConfig({ ...config, api_key: e.target.value })}
            placeholder="sk-..."
            className="w-full px-3 py-2 border rounded-lg"
          />
        </div>

        <div className="border-t pt-4 mt-4">
          <h2 className="text-lg font-semibold mb-3">模型配置</h2>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            事件提取模型
          </label>
          <input
            type="text"
            value={config.extract_model}
            onChange={(e) => setConfig({ ...config, extract_model: e.target.value })}
            placeholder="qwen3:8b"
            className="w-full px-3 py-2 border rounded-lg"
          />
          <p className="text-sm text-gray-500 mt-1">
            用于从文字中提取事件信息，建议用本地小模型
          </p>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            建议生成模型
          </label>
          <input
            type="text"
            value={config.advice_model}
            onChange={(e) => setConfig({ ...config, advice_model: e.target.value })}
            placeholder="deepseek-chat"
            className="w-full px-3 py-2 border rounded-lg"
          />
          <p className="text-sm text-gray-500 mt-1">
            用于生成沟通建议，可用更强的模型
          </p>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            Embedding 模型
          </label>
          <input
            type="text"
            value={config.embed_model}
            onChange={(e) => setConfig({ ...config, embed_model: e.target.value })}
            placeholder="nomic-embed-text"
            className="w-full px-3 py-2 border rounded-lg"
          />
          <p className="text-sm text-gray-500 mt-1">
            用于生成向量 embedding
          </p>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            Embedding 维度
          </label>
          <input
            type="number"
            value={config.embed_dim}
            onChange={(e) => setConfig({ ...config, embed_dim: parseInt(e.target.value) || 768 })}
            className="w-full px-3 py-2 border rounded-lg"
          />
          <p className="text-sm text-gray-500 mt-1">
            nomic-embed-text: 768 / text-embedding-3-small: 1536
          </p>
        </div>
      </div>

      <div className="mt-6 flex items-center gap-4">
        <button
          onClick={handleSave}
          disabled={saving}
          className="bg-primary text-white px-6 py-2 rounded-lg hover:opacity-90 disabled:opacity-50"
        >
          {saving ? '保存中...' : '保存配置'}
        </button>
        {message && (
          <span className={message.includes('失败') ? 'text-red-500' : 'text-green-500'}>
            {message}
          </span>
        )}
      </div>
    </div>
  );
}