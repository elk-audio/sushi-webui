import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';
import { connect } from './lib/net/socket';

connect();
mount(App, { target: document.getElementById('root')! });
