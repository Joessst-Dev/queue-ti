export function inputValue(e: Event): string {
  return (e.target as HTMLInputElement).value;
}

export function selectValue(e: Event): string {
  return (e.target as HTMLSelectElement).value;
}
