// QuestLog — FF-style pixel avatar (Record Keeper proportions: big head, outlined).
// Drawn with box-shadow, 2 frames per mood. Usage: <div data-sprite="cheer|idle|sad" data-px="3"></div>
(function () {
  const PAL = {
    o: '#2b2320', // outline
    h: '#a06a38', H: '#d29a58', // hair + highlight
    s: '#f6cfa2', S: '#d8a878', // skin + shade
    e: '#26203a', // eyes
    t: '#3f6d8e', // tunic
    b: '#d9b45f', // belt
    p: '#5a4632', // pants
    k: '#3a2c20', // boots
    '*': '#8fc7e8' // tear
  };
  const BLANK = '................';

  const IDLE1 = [
    '....oooooooo....',
    '...ohhhhhhhho...',
    '..ohhHHhhhhhho..',
    '..ohHHhhhhhhho..',
    '..ohhhhhhhhhho..',
    '..ohssssssssho..',
    '..ohsessssesho..',
    '..osssssssssso..',
    '...oSssssssSo...',
    '.....oossoo.....',
    '...otttttttto...',
    '..otttttttttto..',
    '..osttttttttso..',
    '..osttbbbbttso..',
    '...otttttttto...',
    '...otttttttto...',
    '....oppooppo....',
    '....oppooppo....',
    '...okko..okko...',
    '...oooo..oooo...'
  ];
  const IDLE2 = [BLANK].concat(IDLE1.slice(0, 19));

  const CHEER1 = [
    '....oooooooo....',
    '...ohhhhhhhho...',
    '..ohhHHhhhhhho..',
    's.ohHHhhhhhhho.s',
    't.ohhhhhhhhhho.t',
    't.ohssssssssho.t',
    't.ohsessssesho.t',
    'o.osssssssssso.o',
    '.o.oSssssssSo.o.',
    '.....oossoo.....',
    '...otttttttto...',
    '..otttttttttto..',
    '..otttttttttto..',
    '..otttbbbbttto..',
    '...otttttttto...',
    '...otttttttto...',
    '....oppooppo....',
    '....oppooppo....',
    '...okko..okko...',
    '...oooo..oooo...'
  ];
  const CHEER2 = [BLANK].concat(CHEER1.slice(0, 19));

  const SAD1 = [
    BLANK,
    '....oooooooo....',
    '...ohhhhhhhho...',
    '..ohhHHhhhhhho..',
    '..ohHHhhhhhhho..',
    '..ohhhhhhhhhho..',
    '..ohssssssssho..',
    '..ohsSssssSsho..',
    '..osssssssssso..',
    '...oSssssssSo...',
    '.....oossoo.....',
    '...otttttttto...',
    '..otttttttttto..',
    '..osttttttttso..',
    '..osttbbbbttso..',
    '...otttttttto...',
    '....oppooppo....',
    '....oppooppo....',
    '...okko..okko...',
    '...oooo..oooo...'
  ];
  const SAD2 = SAD1.map((row, i) => (i === 9 ? '...oSssssssSo*..' : row));

  const ANIMS = { cheer: [CHEER1, CHEER2], idle: [IDLE1, IDLE2], sad: [SAD1, SAD2] };

  function toShadow(map, px) {
    const out = [];
    map.forEach((row, y) => {
      [...row].forEach((ch, x) => {
        if (PAL[ch]) out.push(`${x * px}px ${(y + 1) * px}px 0 ${PAL[ch]}`);
      });
    });
    return out.join(',');
  }

  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;

  document.querySelectorAll('[data-sprite]').forEach((el) => {
    const anim = ANIMS[el.dataset.sprite] || ANIMS.idle;
    const px = +(el.dataset.px || 3);
    el.style.position = 'relative';
    el.style.width = 16 * px + 'px';
    el.style.height = 21 * px + 'px';
    const inner = document.createElement('div');
    inner.style.cssText = `position:absolute;top:0;left:0;width:${px}px;height:${px}px`;
    el.appendChild(inner);
    const frames = anim.map((m) => toShadow(m, px));
    let i = 0;
    inner.style.boxShadow = frames[0];
    if (!reduced) {
      setInterval(() => {
        i = (i + 1) % frames.length;
        inner.style.boxShadow = frames[i];
      }, 430);
    }
  });
})();
