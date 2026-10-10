"""Apply narrowly checked cooperative hooks to the pinned TinyCrypt sources.

Keep the vendor checkout intact. Both firmware and host integration build the
same generated source, including all upstream copyright and license text.
"""
from pathlib import Path


def prepare_crypto(nimble: Path, output: Path) -> list[Path]:
    source = nimble / 'ext/tinycrypt/src'
    ecc = (source / 'ecc.c').read_text()
    boundary = 'for (i = num_bits - 2; i > 0; --i) {'
    if ecc.count(boundary) != 1:
        raise ValueError('Pinned ECC scalar loop changed; review cooperative hook')
    ecc = ecc.replace(boundary, boundary + '\n\t\tGOPINE_ECC_YIELD();')
    # One fixed service point per scalar bit. Never dispatch host callbacks
    # from inside its SM procedure; the NPL controller pump enforces that.
    ecc = ('#ifndef GOPINE_ECC_YIELD\n'
           '#define GOPINE_ECC_YIELD gopine_ble_controller_pump\n'
           '#endif\nvoid GOPINE_ECC_YIELD(void);\n' + ecc)
    ecc_path = output / 'ecc.c'
    ecc_path.write_text(ecc)
    aes = (source / 'aes_encrypt.c').read_text()
    shifts = {'sbox[((a) >> (o))&0xff] << (o)':
              '(unsigned int)sbox[((a) >> (o))&0xff] << (o)',
              '(k[Nb*i]<<24)': '((unsigned int)k[Nb*i]<<24)'}
    # Byte promotion to signed int makes these bit-31 shifts undefined. Keep
    # key expansion identical while checking the complete exchange with UBSan.
    for old, new in shifts.items():
        if aes.count(old) != 1:
            raise ValueError('Pinned AES shifts changed; review unsigned conversions')
        aes = aes.replace(old, new)
    aes_path = output / 'aes_encrypt.c'
    aes_path.write_text(aes)
    return [aes_path, source / 'utils.c', source / 'cmac_mode.c', ecc_path,
            source / 'ecc_dh.c']
