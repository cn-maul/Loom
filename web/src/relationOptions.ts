/**
 * 新建/编辑人物时的常用关系标签。
 *
 * 刻意不放「上级 / 下属」这类方向性关系：同一个人对不同人可能是上级也可能
 * 是同事，方向性关系应该用关系图谱里的结构化关系边（person_relationships）
 * 标注，而不是贴在人物档案的 relation 标签上。
 */
export const RELATION_OPTIONS = ['朋友', '家人', '同学', '同事', '其他'] as const;
