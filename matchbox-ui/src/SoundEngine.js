
class SoundEngine {
  constructor() {
    this.ctx = null;
    this.enabled = true;
    this.initialized = false;
  }

  init() {
    if (this.initialized) return;
    try {
      this.ctx = new (window.AudioContext || window.webkitAudioContext)();
      this.initialized = true;
    } catch (e) {
      console.warn('Web Audio API not supported', e);
    }
  }

  async resume() {
    if (this.ctx && this.ctx.state === 'suspended') {
      await this.ctx.resume();
    }
  }

  setEnabled(val) {
    this.enabled = val;
    if (val && !this.initialized) {
      this.init();
    }
  }

  _playTone(type, startFreq, endFreq, duration, volume = 0.1) {
    if (!this.enabled || !this.ctx) return;
    this.resume(); // Ensure context is running (overcome autoplay restrictions)

    try {
      const osc = this.ctx.createOscillator();
      const gain = this.ctx.createGain();

      osc.connect(gain);
      gain.connect(this.ctx.destination);

      osc.type = type;
      osc.frequency.setValueAtTime(startFreq, this.ctx.currentTime);
      if (endFreq) {
        osc.frequency.exponentialRampToValueAtTime(endFreq, this.ctx.currentTime + (duration * 0.5));
      }

      // Envelope: instant attack, exponential decay
      gain.gain.setValueAtTime(volume, this.ctx.currentTime);
      gain.gain.exponentialRampToValueAtTime(0.001, this.ctx.currentTime + duration);

      osc.start(this.ctx.currentTime);
      osc.stop(this.ctx.currentTime + duration);
    } catch (e) {
      console.warn('Error playing tone', e);
    }
  }

  playMatchChime() {
    // 800Hz to 1200Hz sine wave (Order Match)
    this._playTone('sine', 800, 1200, 0.3, 0.15);
  }

  playThud() {
    // 150Hz to 40Hz bass drop (Whale Wall breach or 2% sudden drop)
    // Triangle wave adds a bit more harmonic content for a 'thud' feeling than pure sine
    this._playTone('triangle', 150, 40, 0.5, 0.4);
  }

  playError() {
    this._playTone('square', 200, 100, 0.4, 0.2);
  }

  playClick() {
    this._playTone('sine', 1500, 1000, 0.05, 0.1);
  }
}

export const soundEngine = new SoundEngine();
