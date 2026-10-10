import asyncio
import struct
import unittest
import direct_upload as direct

class Client:
    def __init__(self, size=500, offset=0, session=0, state=direct.WAITING):
        self.total,self.offset,self.session,self.state=size,offset,session,state
        self.writes=[]
    async def read_gatt_char(self,char):
        return struct.pack('<BBBBIII',1,self.state,0,244,self.total,self.offset,self.session)
    async def write_gatt_char(self,char,data,response):
        assert response
        self.writes.append((char,bytes(data)))
        if char==direct.CONTROL:
            if data[0]==1:
                self.session,self.total=struct.unpack_from('<II',data,1);self.offset=0;self.state=direct.RECEIVING
            elif data[0]==2:self.state=direct.READY
            else:raise AssertionError('Remote install/cancel command')
        else:
            session,offset=struct.unpack_from('<II',data)
            assert session==self.session and offset==self.offset
            self.offset+=len(data)-8

class Tests(unittest.IsolatedAsyncioTestCase):
    async def test_transfer_and_no_remote_install(self):
        c=Client();progress=[]
        await direct.send_connected(c,bytes(500),42,lambda d,t:progress.append(d))
        self.assertEqual(c.offset,500)
        self.assertEqual(progress[-1],500)
        self.assertEqual([p[0] for char,p in c.writes if char==direct.CONTROL],[1,2])
    async def test_resume_at_acknowledged_offset(self):
        c=Client(offset=192,session=42,state=direct.RECEIVING)
        await direct.send_connected(c,bytes(500),42,lambda d,t:None)
        char,data=c.writes[0]
        self.assertEqual(char,direct.DATA)
        self.assertEqual(struct.unpack_from('<I',data,4)[0],192)
    async def test_wrong_session_is_never_overwritten(self):
        c=Client(session=43,state=direct.RECEIVING)
        with self.assertRaises(direct.Rejected):
            await direct.send_connected(c,bytes(500),42,lambda d,t:None)
        self.assertFalse(c.writes)
    async def test_already_verified_does_not_resend(self):
        c=Client(offset=500,session=42,state=direct.READY)
        await direct.send_connected(c,bytes(500),42,lambda d,t:None)
        self.assertFalse(c.writes)
    def test_reject_invalid_status(self):
        for data in (b'',bytes(16),struct.pack('<BBBBIII',1,2,0,244,100,101,42),struct.pack('<BBBBIII',1,5,1,244,100,50,42)):
            with self.assertRaises(direct.Rejected):direct.decode_status(data)


class HostStateTests(unittest.TestCase):
    def test_direct_success_waits_for_user_install(self):
        import pathlib
        import tempfile
        from unittest.mock import Mock, patch
        import ota_update
        with tempfile.TemporaryDirectory() as directory:
            job=ota_update.UploadJob(pathlib.Path('candidate.zip'),'hash','0.3.13',{'address':'C9:9E:15:7A:69:B4','adapter':'hci1'},direct=True)
            process=Mock();process.stdout=iter(['Upload 100% (100/100 bytes)\n']);process.wait.return_value=0
            context=Mock();context.__enter__=Mock(return_value=process);context.__exit__=Mock(return_value=False)
            with patch.object(ota_update,'OUTPUT',pathlib.Path(directory)),patch.object(ota_update.subprocess,'Popen',return_value=context) as launch:
                self.assertTrue(job.start());job.worker.join()
                self.assertEqual(job.state,'awaiting_install')
                self.assertIn('--session',launch.call_args.args[0])
                self.assertFalse(job.start())
                self.assertTrue(job.confirm())

if __name__ == '__main__':
    unittest.main()
