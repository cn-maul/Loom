import { useState, useEffect } from 'react';
import { personApi } from '../api/client';
import type { Person } from '../api/types';

export default function Home() {
  const [persons, setPersons] = useState<Person[]>([]);
  const [selectedPerson, setSelectedPerson] = useState<Person | null>(null);
  const [newPersonName, setNewPersonName] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadPersons();
  }, []);

  const loadPersons = async () => {
    try {
      const data = await personApi.list();
      setPersons(data || []);
    } catch (error) {
      console.error('Failed to load persons:', error);
    }
  };

  const handleCreatePerson = async () => {
    if (!newPersonName.trim()) return;

    setLoading(true);
    try {
      await personApi.create({
        name: newPersonName,
        relation: '朋友',
        importance: 3,
      });
      setNewPersonName('');
      await loadPersons();
    } catch (error) {
      console.error('Failed to create person:', error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-6xl mx-auto p-4">
      <div className="grid grid-cols-12 gap-6">
        {/* 左侧：人物列表 */}
        <div className="col-span-3 bg-white rounded-lg shadow p-4">
          <h2 className="text-lg font-semibold mb-4">人物列表</h2>
          
          <div className="mb-4">
            <input
              type="text"
              value={newPersonName}
              onChange={(e) => setNewPersonName(e.target.value)}
              placeholder="输入姓名"
              className="w-full px-3 py-2 border rounded-lg mb-2"
              onKeyPress={(e) => e.key === 'Enter' && handleCreatePerson()}
            />
            <button
              onClick={handleCreatePerson}
              disabled={loading}
              className="w-full bg-primary text-white py-2 rounded-lg hover:opacity-90 disabled:opacity-50"
            >
              {loading ? '创建中...' : '新建人物'}
            </button>
          </div>

          <div className="space-y-2">
            {persons.map((person) => (
              <div
                key={person.id}
                onClick={() => setSelectedPerson(person)}
                className={`p-3 rounded-lg cursor-pointer transition-colors ${
                  selectedPerson?.id === person.id
                    ? 'bg-primary/10 border border-primary'
                    : 'bg-gray-50 hover:bg-gray-100'
                }`}
              >
                <div className="font-medium">{person.name}</div>
                <div className="text-sm text-gray-500">{person.relation}</div>
              </div>
            ))}
          </div>
        </div>

        {/* 右侧：详情区域 */}
        <div className="col-span-9 bg-white rounded-lg shadow p-4">
          {selectedPerson ? (
            <div>
              <h2 className="text-2xl font-bold mb-4">{selectedPerson.name}</h2>
              <p className="text-gray-600 mb-4">{selectedPerson.notes || '暂无备注'}</p>
              
              <div className="border-t pt-4">
                <h3 className="text-lg font-semibold mb-2">事件时间线</h3>
                <p className="text-gray-500">请在下方实现事件记录功能</p>
              </div>
            </div>
          ) : (
            <div className="text-center text-gray-500 py-20">
              请从左侧选择一个人物
            </div>
          )}
        </div>
      </div>
    </div>
  );
}