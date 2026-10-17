import { mount } from 'svelte';
import App from './App.svelte';
import './styles/app.css';

// Every font face loads before the first paint. A face otherwise loads when text first
// uses it, and text drawn in the fallback until then moves when it swaps in. The faces
// are local and small, so this takes a few ms; a face that fails never holds the app.
await Promise.race([
  Promise.all([...document.fonts].map((f) => f.load().catch(() => {}))),
  new Promise((done) => setTimeout(done, 500)),
]);

export default mount(App, { target: document.getElementById('app')! });
