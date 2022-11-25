package ca.concordia;

import junitparams.JUnitParamsRunner;
import junitparams.Parameters;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.junit.runners.Parameterized;

import java.net.InetAddress;
import java.util.Arrays;
import java.util.Collection;

import static org.assertj.core.api.Assertions.assertThat;


@RunWith(JUnitParamsRunner.class)
public class PacketTest {

    @Test
    @Parameters(method = "packets")
    public void testWritePacket(Packet p, int[] expectInts) throws Exception {
        byte[] expectBytes = new byte[expectInts.length];
        for(int i = 0; i < expectBytes.length; i++){
            expectBytes[i] = (byte)expectInts[i];
        }
        assertThat(p.toBytes())
                .isEqualTo(expectBytes);
    }


    @Test
    @Parameters(method = "packets")
    public void testParseValidPackets(Packet expectPacket, int[] inputInts) throws Exception {
        byte[] inputBytes = new byte[inputInts.length];
        for(int i = 0; i < inputBytes.length; i++){
            inputBytes[i] = (byte)inputInts[i];
        }
        Packet p = Packet.fromBytes(inputBytes);

        assertThat(p.toBytes())
                .isEqualTo(inputBytes);

        assertThat(p.toBytes())
                .isEqualTo(expectPacket.toBytes());
    }

    public static InetAddress addr(int ...vals) throws Exception{
        assert vals.length == 4;
        byte[] bytes = new byte[4];
        for(int i = 0; i < vals.length; i++){
            bytes[i] = (byte)vals[i];
        }
        return InetAddress.getByAddress(bytes);
    }

    @Parameterized.Parameters
    public static Collection<Object[]> packets() throws Exception{
        return Arrays.asList(new Object[][]{
                {
                        new Packet.Builder()
                                .setType(1)
                                .setSequenceNumber(45)
                                .setPeerAddress(addr(127, 0, 0, 1))
                                .setPortNumber(2100)
                                .setPayload("Hello World".getBytes())
                                .create(),
                        new int[]{
                                1,
                                0, 0, 0, 45,
                                127, 0, 0, 1,
                                8, 52,
                                72,101,108,108,111,32,87,111,114,108,100
                        }
                },
                {
                        new Packet.Builder()
                                .setType(29)
                                .setSequenceNumber(2992122123L) //unsigned int
                                .setPeerAddress(addr(192, 168, 2, 125))
                                .setPortNumber(53201)
                                .setPayload("".getBytes())
                                .create(),
                        new int[]{
                                29,
                                178, 88, 41, 11,
                                192, 168, 2, 125,
                                207, 209,
                        }
                },
        });
    }
}