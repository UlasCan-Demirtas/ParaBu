import { soundEngine } from './SoundEngine';

class ToastEmitter {
  listeners = [];
  add(listener) {
    this.listeners.push(listener);
  }
  remove(listener) {
    this.listeners = this.listeners.filter(l => l !== listener);
  }
  emit(toast) {
    this.listeners.forEach(l => l(toast));
  }
}

export const toastEvent = new ToastEmitter();

export const showToast = (msg, type = 'success', title = null) => {
  if (type === 'success') {
    soundEngine.playClick();
  } else {
    soundEngine.playError();
  }
  toastEvent.emit({ id: Date.now() + Math.random(), msg, type, title });
};
