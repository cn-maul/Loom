import type { Person } from '../api/types';
import { controlClass } from './ui';

interface Props {
  persons: Person[];
  value: string;
  onChange: (id: string) => void;
  placeholder?: string;
}

export default function PersonPicker({ persons, value, onChange, placeholder = '选择人物' }: Props) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={`${controlClass} w-auto text-muted-foreground`}
    >
      <option value="">{placeholder}</option>
      {persons.map((person) => (
        <option key={person.id} value={person.id}>
          {person.name}
          {person.relation ? ` · ${person.relation}` : ''}
        </option>
      ))}
    </select>
  );
}
